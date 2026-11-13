#!/usr/bin/env python3
"""Isolated real-process nginx-style mount/bootstrap/upgrade/shutdown regression."""
import argparse
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time

ROOT = Path(__file__).resolve().parent.parent


def check(binary):
    with tempfile.TemporaryDirectory(prefix='hgw-deployment-check-') as temporary:
        root = Path(temporary)
        data = root / 'gateway'; data.mkdir(mode=0o700)
        initial = root / 'initial.json'
        config = root / 'startup.json'
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
        config.write_text(json.dumps({'trusted_proxies': ['127.0.0.1'], 'allow_frame': True, 'base_path': '/gateway',
            'listen_addr': f'127.0.0.1:{port}', 'node_name': 'isolated-gateway',
            'initial_admin_file': str(initial)})); config.chmod(0o600)
        child = None
        logs = root / 'process.log'

        class Proxy(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def forward(self):
                if not self.path.startswith('/gateway/'):
                    self.send_error(404); return
                connection = http.client.HTTPConnection('127.0.0.1', port, timeout=10)
                headers = dict(self.headers)
                headers['X-Forwarded-Proto'] = 'https'
                body = self.rfile.read(int(self.headers.get('Content-Length', '0')))
                try:
                    connection.request(self.command, self.path[len('/gateway'):], body, headers)
                    response = connection.getresponse(); content = response.read()
                except (OSError, http.client.HTTPException):
                    connection.close(); self.send_error(503); return
                self.send_response(response.status)
                for key, value in response.getheaders():
                    if key.lower() not in ('connection', 'transfer-encoding', 'content-length'):
                        self.send_header(key, value)
                self.send_header('Content-Length', str(len(content)))
                self.end_headers(); self.wfile.write(content); connection.close()

            do_GET = forward
            do_POST = forward
            do_PUT = forward

        proxy = ThreadingHTTPServer(('127.0.0.1', 0), Proxy)
        thread = threading.Thread(target=proxy.serve_forever, daemon=True); thread.start()
        origin = f'http://127.0.0.1:{proxy.server_port}'

        def request(path, method='GET', body=None, cookie='', csrf=''):
            connection = http.client.HTTPConnection('127.0.0.1', proxy.server_port, timeout=10)
            headers = {'Origin': origin, 'Content-Type': 'application/json'}
            if cookie: headers['Cookie'] = cookie
            if csrf: headers['X-CSRF-Token'] = csrf
            connection.request(method, '/gateway' + path,
                               None if body is None else json.dumps(body), headers)
            response = connection.getresponse(); content = response.read()
            result = response.status, dict(response.getheaders()), content
            connection.close(); return result

        def start():
            nonlocal child
            initial.write_text(json.dumps({'username': 'admin', 'password': 'initial-password-123'}))
            initial.chmod(0o600)
            with logs.open('ab') as output:
                child = subprocess.Popen([str(binary), '--db', str(data / 'gateway.sqlite')],
                    env=dict(os.environ, GATEWAY_BOOTSTRAP_FILE=str(config)), stdout=output, stderr=output)
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                if child.poll() is not None:
                    raise AssertionError('isolated Gateway stopped during startup')
                try:
                    if request('/api/v1/auth/methods')[0] == 200: return
                except (OSError, http.client.HTTPException): pass
                time.sleep(.05)
            raise AssertionError('Gateway did not start')

        def stop():
            if child is not None and child.poll() is None:
                child.terminate()
                child.wait(timeout=10)
                assert child.returncode == 0, 'SIGTERM was not handled cleanly'

        def login(password):
            return request('/api/v1/auth/login', 'POST', {'username':'admin', 'password':password})

        try:
            start()
            assert request('/')[1]['Location'] == '/gateway/admin/'
            status, headers, html = request('/admin/')
            assert status == 200 and headers['X-Frame-Options'] == 'SAMEORIGIN'
            import re
            for asset in re.findall(rb'(?:src|href)="([a-zA-Z0-9_.-]+\.(?:js|css))"', html):
                assert request('/admin/' + asset.decode())[0] == 200, asset
            status, headers, raw = login('initial-password-123')
            assert status == 200
            cookie = headers['Set-Cookie'].split(';')[0]
            assert 'Path=/gateway/' in headers['Set-Cookie'] and 'Secure' in headers['Set-Cookie']
            csrf = json.loads(raw)['csrf']
            assert request('/api/v1/status', cookie=cookie)[0] == 200
            assert request('/api/v1/reload', 'POST', {}, cookie=cookie)[0] == 401
            status, _, _ = request('/api/v1/auth/settings', 'PUT', {
                'mode': 'local', 'local_username': 'admin', 'local_password': 'changed-password-123',
            }, cookie, csrf)
            assert status == 200
            key_hash = hashlib.sha256((data / 'master.key').read_bytes()).hexdigest()
            stop(); start()
            assert login('initial-password-123')[0] == 401, 'installer reset an existing password'
            assert login('changed-password-123')[0] == 200
            assert hashlib.sha256((data / 'master.key').read_bytes()).hexdigest() == key_hash
            assert not initial.exists(), 'initial password file was retained'
            stop()
            with sqlite3.connect(data / 'gateway.sqlite') as db:
                assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
            for secret in (b'initial-password-123', b'changed-password-123'):
                assert secret not in logs.read_bytes(), 'password leaked to logs'
            print('PASS: real proxy mount/assets, headless login, CSRF, credential-preserving upgrade, stable key, clean SIGTERM and SQLite integrity')
        finally:
            stop(); proxy.shutdown(); proxy.server_close(); thread.join(timeout=5)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path)
    args = parser.parse_args()
    if args.binary:
        check(args.binary.resolve())
    else:
        with tempfile.TemporaryDirectory(prefix='hgw-deployment-bin-') as directory:
            binary = Path(directory) / 'headscale-gateway'
            subprocess.run(['go', 'build', '-buildvcs=false', '-o', str(binary), '.'], cwd=ROOT / "src",
                           env=dict(os.environ, CGO_ENABLED='0'), check=True)
            check(binary)
