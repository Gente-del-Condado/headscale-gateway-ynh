#!/usr/bin/env python3
"""Prueba de recuperación con PTY en Linux/macOS y base temporal, sin datos productivos."""
import json, os, pty, re, select, subprocess, tempfile, time, urllib.request, urllib.error
from pathlib import Path
ROOT = Path(__file__).resolve().parent.parent

def run(binary, db, terminal):
    master, slave = pty.openpty() if terminal else os.pipe()
    env = dict(os.environ, HGW_CONSOLE_FIXTURE_DB=str(db), GATEWAY_LANG='en', TERM='xterm-256color')
    env.pop('NO_COLOR', None)
    child = subprocess.Popen([str(binary), '-test.run=^TestConsoleIntegrationProcess$'],
                             stdout=slave, stderr=slave, env=env, cwd=ROOT / "src")
    os.close(slave)
    output = b''
    try:
        deadline = time.monotonic()+20
        while time.monotonic()<deadline:
            if select.select([master], [], [], .2)[0]:
                output += os.read(master, 65536)
                if b'Recovery does not automatically' in output:
                    break
            if child.poll() is not None: raise AssertionError('fixture stopped before banner')
        assert b'PANEL ACCESS' in output, 'missing startup banner'
        assert (b'\x1b[1;96m' in output) == terminal, 'ANSI terminal detection'
        if terminal:
            match = re.search(rb'\x1b\[1;96m([a-zA-Z0-9-]{8,})\x1b\[0m', output)
            assert match, 'missing highlighted code'
            code = match[1].decode()
            origin = re.search(rb'http://127\.0\.0\.1:\d+', output)[0].decode()
            payload = json.dumps({'code': code}).encode()
            request = urllib.request.Request(origin+'/api/v1/auth/bootstrap', data=payload, headers={'Content-Type':'application/json'})
            with urllib.request.urlopen(request) as response:
                assert json.load(response).get('token'), 'bootstrap did not yield recovery credential'
            try:
                urllib.request.urlopen(request)
                raise AssertionError('bootstrap code accepted twice')
            except urllib.error.HTTPError as error:
                assert error.code in (400,401,403), 'unexpected replay rejection'
            for log in db.parent.rglob('*.log'):
                assert code.encode() not in log.read_bytes(), 'code leaked to file log'
        else:
            assert b'TEMPORARY RECOVERY CODE' not in output, 'secret banner in redirected output'
            assert b'Code pending:' in output, 'recovery not deferred'
    finally:
        child.terminate()
        try: child.wait(timeout=5)
        except subprocess.TimeoutExpired: child.kill(); child.wait()
        os.close(master)

with tempfile.TemporaryDirectory(prefix='hgw-console-check-') as temp:
    base = Path(temp)
    binary = base/'console.test'
    subprocess.run(['go','test','-c','-o',str(binary),'.'], cwd=ROOT / "src", check=True, env=dict(os.environ,CGO_ENABLED='0'))
    db = base/'state/gateway.sqlite'
    run(binary, db, False)
    run(binary, db, True)
    print('PASS: redirected recovery stays pending; PTY color/link; English; code works once; no code in file logs.')
