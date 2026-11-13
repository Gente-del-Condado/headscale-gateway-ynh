#!/usr/bin/env python3
"""Ejecutar solo en una imagen desechable del complemento, con Supervisor simulado."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import http.client
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import threading
import time
import urllib.request

assert Path('/run/.containerenv').exists() or Path('/.dockerenv').exists(), 'Disposable container required'
assert not Path('/data/gateway/gateway.sqlite').exists(), 'Use empty disposable container data'
addon_config = Path(sys.argv[1] if len(sys.argv) > 1 else '/addon-config.yaml').read_text()
ingress_entry = re.search(r'^ingress_entry: (.+)$', addon_config, re.M)[1]
Path('/data').mkdir(exist_ok=True)
Path('/data/options.json').write_text(json.dumps({
    'node_name': 'container-test', 'lan_port': 18443,
}))


class Supervisor(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        assert self.path == '/addons/self/info'
        assert self.headers['Authorization'] == 'Bearer isolated-supervisor-token'
        value = json.dumps({'result': 'ok', 'data': {'ingress_port': 63001, 'ingress': True, 'ingress_panel': False}}).encode()
        self.send_response(200); self.send_header('Content-Length', str(len(value)))
        self.end_headers(); self.wfile.write(value)


server = ThreadingHTTPServer(('0.0.0.0', 80), Supervisor)
thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
logs = Path('/tmp/gateway-container.log')
with logs.open('wb') as output:
    child = subprocess.Popen(['python3', '/usr/local/lib/headscale-gateway/run.py'],
        env=dict(os.environ, SUPERVISOR_TOKEN='isolated-supervisor-token'), stdout=output, stderr=output)
try:
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        assert child.poll() is None, 'Container entry point stopped unexpectedly'
        match = re.search(r'Panel disponible mediante el proxy autorizado; escucha interna: (?:0\.0\.0\.0|\[::\]):(\d+)', logs.read_text())
        if match:
            break
        time.sleep(.05)
    assert match, 'Administration did not start: ' + logs.read_text()
    text = logs.read_text()
    assert 'ABRIR INTERFAZ WEB' in text and 'MOSTRAR EN LA BARRA LATERAL' in text
    assert 'Abre este enlace en el navegador de este equipo' not in text
    assert not re.search(r'http://(?:0\.0\.0\.0|\[::\]):', text), 'Unusable browser link in app logs'
    headers = {'X-Ingress-Path': '/api/hassio_ingress/test', 'Host': 'ha.test',
               'Origin': 'https://ha.test', 'X-Forwarded-Proto': 'https',
               'X-Remote-User-Id': 'ha-admin-id', 'X-Remote-User-Groups': 'system-admin,system-users'}

    class Ingress(BaseHTTPRequestHandler):
        """Reenvía la ruta del navegador como Core y Supervisor, sin normalizarla."""
        def log_message(self, *_):
            pass

        def do_GET(self):
            prefix = headers['X-Ingress-Path'] + '/'
            if not self.path.startswith(prefix):
                self.send_error(404, 'Request escaped the ingress mount')
                return
            upstream = http.client.HTTPConnection('172.30.32.2', 63001, timeout=10)
            upstream.request('GET', '/' + self.path[len(prefix):], headers=headers)
            result = upstream.getresponse()
            body = result.read()
            self.send_response(result.status)
            for key, value in result.getheaders():
                if key.lower() not in ('transfer-encoding', 'connection', 'content-length'):
                    self.send_header(key, value)
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            upstream.close()

    ingress = ThreadingHTTPServer(('127.0.0.1', 18080), Ingress)
    ingress_thread = threading.Thread(target=ingress.serve_forever, daemon=True)
    ingress_thread.start()
    try:
        # Supervisor añade literalmente el valor del manifiesto a app.ingress_url.
        browser_url = 'http://127.0.0.1:18080/api/hassio_ingress/test/' + ingress_entry
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(browser_url, timeout=10) as page:
            assert page.status == 200 and b'http-paths.js' in page.read()
            assert page.url == browser_url, 'Open Web UI unexpectedly redirected'
        with opener.open('http://127.0.0.1:18080/api/hassio_ingress/test/api/v1/auth/session', timeout=10) as response:
            assert json.load(response)['source'] == 'proxy'
    finally:
        ingress.shutdown(); ingress.server_close(); ingress_thread.join(timeout=5)

    connection = http.client.HTTPConnection('172.30.32.2', int(match[1]), timeout=10)
    connection.request('GET', '/admin/', headers=headers)
    response = connection.getresponse(); html = response.read()
    assert response.status == 200 and response.getheader('X-Frame-Options') == 'SAMEORIGIN'
    assert "frame-ancestors 'self'" in response.getheader('Content-Security-Policy')
    connection.close()
    for asset in re.findall(rb'(?:src|href)="([a-zA-Z0-9_.-]+\.(?:js|css))"', html):
        connection = http.client.HTTPConnection('172.30.32.2', int(match[1]), timeout=10)
        connection.request('GET', '/admin/' + asset.decode(), headers=headers)
        response = connection.getresponse(); response.read()
        assert response.status == 200, asset
        connection.close()
    connection = http.client.HTTPConnection('172.30.32.2', int(match[1]), timeout=10)
    connection.request('GET', '/api/v1/auth/session', headers=headers)
    response = connection.getresponse(); session = json.loads(response.read())
    assert response.status == 200 and session['user'] == 'proxy:ha-admin-id' and session['role'] == 'admin'
    assert session['source'] == 'proxy' and not response.getheader('Set-Cookie'), 'SSO minted a transferable cookie'
    connection.close()
    connection = http.client.HTTPConnection('172.30.32.2', 63001, timeout=10)
    connection.request('GET', '/api/v1/status', headers=headers)
    response = connection.getresponse(); status = json.loads(response.read())
    assert response.status == 200 and status['version'] == Path('/usr/share/headscale-gateway/VERSION').read_text().strip()
    connection.close()
    for body_headers, code in ((dict(headers, **{'X-Remote-User-Groups': 'system-users'}), 403),
                               (dict(headers, **{'X-Remote-User-Id': ''}), 403),
                               (headers, 401),
                               (dict(headers, **{'X-CSRF-Token': session['csrf']}), 400)):
        connection = http.client.HTTPConnection('172.30.32.2', 63001, timeout=10)
        connection.request('POST', '/api/v1/reload', '{}', dict(body_headers, **{'Content-Type':'application/json'}))
        response = connection.getresponse(); body = response.read()
        assert response.status == code, f'SSO/CSRF expected {code}, got {response.status}'
        if code == 400:
            assert b'modo configuraci' in body, 'Valid administrator did not reach the setup-mode controller'
        connection.close()
    try:
        with socket.create_connection(('127.0.0.1', 9091), timeout=1):
            raise AssertionError('An extra administrative listener was opened')
    except ConnectionRefusedError:
        pass
    connection = http.client.HTTPConnection('127.0.0.1', 63001, timeout=10)
    connection.request('GET', '/admin/', headers={'X-Ingress-Path':'/api/hassio_ingress/test'})
    response = connection.getresponse(); response.read()
    assert response.status == 403, 'Ingress accepted a non-Supervisor source'
    connection.close()
    assert b'SUPERVISOR_TOKEN=' not in Path(f'/proc/{child.pid}/environ').read_bytes()
    assert not Path('/run/headscale-gateway/initial-admin.json').exists()
    assert Path('/data/gateway/master.key').stat().st_mode & 0o777 == 0o600
    child.terminate(); child.wait(timeout=10)
    assert child.returncode == 0
    assert 'container-test-password' not in logs.read_text()
    print('PASS: manifest Open Web UI through simulated ingress without escaping redirects; HA administrator SSO without Gateway login/cookie, iframe/assets, CSRF, non-admin and spoof rejection, private data, Supervisor token removal and SIGTERM')
finally:
    if child.poll() is None:
        child.terminate(); child.wait(timeout=10)
    server.shutdown(); server.server_close(); thread.join(timeout=5)
