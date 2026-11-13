#!/usr/bin/env python3
"""Laboratorio desechable con nginx/NSS: simula solo la identidad verificada de SSOwat."""
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import tempfile
import time
import tomllib

assert Path('/run/.containerenv').exists() or Path('/.dockerenv').exists(), 'Disposable container required'
assert not Path('/data/gateway/gateway.sqlite').exists(), 'Do not use existing app data'
fixture = Path('/test-package')
expected = tomllib.loads((fixture/'manifest.toml').read_text())['version'].split('~')[0]
groups = Path('/etc/group')
original_groups = groups.read_text()
assert not any(line.startswith('admins:') for line in original_groups.splitlines()), 'Clean NSS fixture required'


def set_admin(member=True):
    groups.write_text(original_groups + '\nadmins:x:4900:' + ('alice' if member else '') + '\n')


def wait_port(port, process):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        assert process.poll() is None, 'Fixture process exited during startup'
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=.2):
                return
        except OSError:
            time.sleep(.05)
    raise AssertionError('Fixture listener did not start')


def request(port, path, *, cookie='admin-session', method='GET', extra=None):
    connection = http.client.HTTPConnection('127.0.0.1', port, timeout=10)
    headers = {'Host':'ynh.test:18080', 'Origin':'https://ynh.test:18080', 'Content-Type':'application/json'}
    if cookie:
        headers['Cookie'] = 'fixture_session=' + cookie
    headers.update(extra or {})
    connection.request(method, path, '{}' if method == 'POST' else None, headers)
    response = connection.getresponse()
    result = response.status, dict(response.getheaders()), response.read()
    connection.close()
    return result


try:
    with tempfile.TemporaryDirectory(prefix='hgw-ynh-sso-') as temporary:
        root = Path(temporary)
        install, data = root/'install', root/'data'
        include = Path('/etc/nginx/conf.d/ynh.test.d/headscale_gateway-proxy-auth.inc')
        old_secret = None
        old_key = None
        for mount in ('/gateway', ''):
            set_admin()
            subprocess.run(['python3',str(fixture/'conf/configure.py'), '--app','headscale_gateway',
                '--install-dir',str(install),'--data-dir',str(data),'--port','19091',
                '--lan-port','18443','--path',mount or '/','--proxy-include',str(include)],check=True)
            startup = json.loads((install/'startup.json').read_text())
            secret = startup['proxy_auth']['shared_secret']
            assert 'initial_admin_file' not in startup, 'Gateway password required for SSO'
            assert include.stat().st_mode & 0o777 == 0o600 and include.stat().st_uid == 0
            if old_secret:
                assert old_secret == secret, 'URL change rotated proxy identity unexpectedly'
            old_secret = secret
            template = (fixture/'conf/nginx.conf').read_text()
            for name, value in {'APP':'headscale_gateway','DOMAIN':'ynh.test','PATH':mount,'PORT':'19091'}.items():
                template = template.replace('__'+name+'__',value)
            # Simula el filtrado de cabeceras de SSOwat y la cookie autenticada del portal.
            # Conserva la ubicación nginx real de la aplicación y su prueba privada.
            config = 'pid ' + str(root/'nginx.pid') + ';\nerror_log stderr warn;\n' + '''include /etc/nginx/modules-enabled/*.conf;
events {}
http {
    access_log off;
    server {
        listen 127.0.0.1:18080;
        access_by_lua_block {
            for name, _ in pairs(ngx.req.get_headers()) do
                local lower = string.lower(name)
                if string.sub(lower,1,4) == "ynh_" or string.sub(lower,1,4) == "ynh-" then
                    ngx.req.clear_header(name)
                end
            end
            local session = ngx.var.cookie_fixture_session
            if session == "admin-session" then
                ngx.req.set_header("YNH_USER", "alice")
            elseif session == "user-session" then
                ngx.req.set_header("YNH_USER", "bob")
            else
                return ngx.redirect("/yunohost/sso/", 302)
            end
        }
''' + template + '\n}\n}\n'
            nginx_config = root/'nginx.conf'
            nginx_config.write_text(config)
            subprocess.run(['nginx','-t','-c',str(nginx_config)],check=True)
            gateway = nginx = None
            logs = root/'gateway.log'
            nginx_logs = root/'nginx.log'
            try:
                with logs.open('wb') as output:
                    gateway = subprocess.Popen(['/usr/local/bin/headscale-gateway','--db',str(data/'gateway/gateway.sqlite')],
                        env=dict(os.environ,GATEWAY_BOOTSTRAP_FILE=str(install/'startup.json')),stdout=output,stderr=output)
                wait_port(19091,gateway)
                with nginx_logs.open('wb') as output:
                    nginx = subprocess.Popen(['nginx','-c',str(nginx_config),'-g','daemon off;'],stdout=output,stderr=output)
                wait_port(18080,nginx)
                status, headers, raw = request(18080,mount+'/api/v1/auth/session')
                session = json.loads(raw)
                assert status == 200 and session['user'] == 'proxy:alice' and session['role'] == 'admin'
                assert 'Set-Cookie' not in headers, 'Delegation minted a portable login cookie'
                assert request(18080,mount+'/api/v1/status')[0] == 200
                status, _, raw = request(18080,mount+'/api/v1/status')
                assert json.loads(raw)['version'] == expected
                assert request(18080,mount+'/api/v1/auth/session',cookie='')[0] == 302
                assert request(18080,mount+'/api/v1/auth/session',cookie='user-session',extra={
                    'Ynh-User':'alice','YNH_USER':'alice','X-Remote-User-Groups':'admins'})[0] == 403
                assert request(19091,'/api/v1/auth/session',extra={'Ynh-User':'alice'})[0] == 403
                assert request(18080,mount+'/api/v1/reload',method='POST')[0] == 401
                assert request(18080,mount+'/api/v1/reload',method='POST',extra={'X-CSRF-Token':session['csrf']})[0] == 400
                assert request(18080,mount+'/api/v1/reload',method='POST',extra={
                    'X-CSRF-Token':session['csrf'],'Origin':'https://evil.test'})[0] == 403
                status, headers, html = request(18080,mount+'/admin/')
                assert status == 200 and headers['X-Frame-Options'] == 'SAMEORIGIN'
                for asset in re.findall(rb'(?:src|href)="([a-zA-Z0-9_.-]+\.(?:js|css))"',html):
                    assert request(18080,mount+'/admin/'+asset.decode())[0] == 200, asset
                set_admin(False)
                assert request(18080,mount+'/api/v1/auth/session',extra={'X-CSRF-Token':session['csrf']})[0] == 403
                key = hashlib.sha256((data/'gateway/master.key').read_bytes()).hexdigest()
                assert old_key is None or key == old_key, 'URL change changed Gateway identity key'
                old_key = key
            finally:
                for process in (nginx,gateway):
                    if process and process.poll() is None:
                        process.terminate(); process.wait(timeout=10)
                if gateway:
                    assert gateway.returncode == 0
            assert secret not in logs.read_text() and secret not in nginx_logs.read_text(), 'Private proxy proof leaked to logs'
        print('PASS: real nginx/NSS, simulated SSOwat identity, admin SSO without password, root/subpath, spoof rejection, CSRF, immediate group revocation, stable key and URL-change proof')
finally:
    groups.write_text(original_groups)
