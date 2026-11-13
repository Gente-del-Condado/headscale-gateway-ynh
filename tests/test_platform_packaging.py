import importlib.util
import hashlib
import io
import json
from pathlib import Path
import os
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'packaging/common'))


class PlatformPackagingTest(unittest.TestCase):
    def test_yunohost_restore_repairs_private_startup_and_rejects_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            startup = root / 'startup.json'
            startup.write_text('{"proxy_auth":{"shared_secret":"fixture"}}')
            startup.chmod(0o644)
            original = startup.read_bytes()
            command = ['bash', '-c', '''set -euo pipefail
source "$1"
install_dir="$2"; app=fixture
ynh_die() { exit 90; }
chown() { [[ "$1" == --no-dereference && "$2" == fixture:fixture && "$3" == "$install_dir/startup.json" ]]; }
hgw_restore_startup_permissions
''', 'restore-test', str(ROOT/'packaging/yunohost/scripts/_common.sh'), str(root)]
            subprocess.run(command, check=True)
            self.assertEqual(startup.stat().st_mode & 0o777, 0o600)
            self.assertEqual(startup.read_bytes(), original)
            target = root/'external.json'
            startup.rename(target)
            target.chmod(0o644)
            startup.symlink_to(target)
            self.assertEqual(subprocess.run(command).returncode, 90)
            self.assertEqual(target.stat().st_mode & 0o777, 0o644)

    def test_yunohost_logo_registers_the_existing_icon_and_preserves_customization(self):
        logo = ROOT / 'yunohost/doc/logo.png'
        self.assertEqual(logo.read_bytes(), (ROOT / 'packaging/homeassistant/icon.png').read_bytes())
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            state = root / 'permission.json'
            record = root / 'calls.json'
            fake_yunohost = root / 'yunohost'
            fake_yunohost.write_text('''#!/usr/bin/env python3
import hashlib, json, os, pathlib, sys
state = pathlib.Path(os.environ['HGW_TEST_PERMISSION'])
record = pathlib.Path(os.environ['HGW_TEST_CALLS'])
args = sys.argv[1:]
assert args[:2] == ['user', 'permission'], args
assert args[3] == 'headscale_gateway__2.main', args
if args[2] == 'info':
    print(state.read_text())
elif args[2] == 'update':
    assert len(args) == 5 and args[4].startswith('--logo='), args
    logo = pathlib.Path(args[4].split('=', 1)[1]).read_bytes()
    assert logo.startswith(b'\\x89PNG\\r\\n\\x1a\\n')
    value = json.loads(state.read_text())
    value['logo_hash'] = hashlib.sha256(logo).hexdigest()
    state.write_text(json.dumps(value))
    record.write_text(json.dumps(args))
else:
    raise AssertionError(args)
''')
            fake_yunohost.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                       HGW_TEST_PERMISSION=str(state), HGW_TEST_CALLS=str(record))
            command = ['bash', '-c', 'set -euo pipefail; source "$1"; app=headscale_gateway__2; hgw_install_logo "$2"',
                       'logo-test', str(ROOT / 'packaging/yunohost/scripts/_common.sh'), str(logo)]
            original = {'allowed': ['admins'], 'url': '/', 'auth_header': True, 'show_tile': True}
            state.write_text(json.dumps(original))
            subprocess.run(command, check=True, env=env)
            expected_hash = hashlib.sha256(logo.read_bytes()).hexdigest()
            self.assertEqual(json.loads(state.read_text()), dict(original, logo_hash=expected_hash))
            self.assertTrue(record.is_file())
            # Al restaurar el logo propio se vuelve a registrar para reponer su caché.
            record.unlink()
            subprocess.run(command, check=True, env=env)
            self.assertTrue(record.is_file())
            custom = dict(original, logo_hash='custom-logo-hash')
            state.write_text(json.dumps(custom))
            record.unlink()
            subprocess.run(command, check=True, env=env)
            self.assertFalse(record.exists())
            self.assertEqual(json.loads(state.read_text()), custom)
            # Una consulta fallida no debe convertirse en un cambio del logo.
            state.write_text('invalid JSON')
            result = subprocess.run(command, env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(record.exists())

    def test_homeassistant_access_instructions_use_native_ui(self):
        spec = importlib.util.spec_from_file_location('hgw_ha_instructions', ROOT / 'packaging/homeassistant/run.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        output = io.StringIO()
        module.print_access_instructions(output)
        text = output.getvalue()
        self.assertIn('ABRIR INTERFAZ WEB', text)
        self.assertIn('MOSTRAR EN LA BARRA LATERAL', text)
        self.assertNotIn('http://', text)

    def test_homeassistant_preparation_preserves_state_and_seals_credentials(self):
        spec = importlib.util.spec_from_file_location('hgw_ha_run', ROOT / 'packaging/homeassistant/run.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data, runtime = root / 'data', root / 'runtime'
            options = {'node_name': 'ha-gateway', 'admin_username': 'admin', 'admin_password': 'a-password-123456'}
            db, deployment = module.prepare(options, 63001, data, runtime)
            db.write_bytes(b'existing-database')
            (db.parent / 'master.key').write_bytes(b'existing-master-key')
            module.prepare(options, 63002, data, runtime)
            self.assertEqual(db.read_bytes(), b'existing-database')
            self.assertEqual((db.parent / 'master.key').read_bytes(), b'existing-master-key')
            value = json.loads(deployment.read_text())
            self.assertEqual(value['node_name'], 'ha-gateway')
            self.assertEqual(value['listen_addr'], '0.0.0.0:63002')
            self.assertEqual(value['proxy_auth']['admin_group'], 'system-admin')
            for path in (deployment, Path(value['initial_admin_file'])):
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertNotIn('a-password-123456', deployment.read_text())
            self.assertFalse((data / 'options.json').exists())
            with self.assertRaises(ValueError):
                module.prepare(dict(options, admin_password='short'), 63001, data, runtime)

    def test_homeassistant_sso_requires_admin_only_ingress_and_no_password(self):
        spec = importlib.util.spec_from_file_location('hgw_ha_sso', ROOT / 'packaging/homeassistant/run.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        # Real /addons/self/info responses omit panel_admin. ingress_panel is
        # only the sidebar preference and may legitimately be disabled.
        good = {'result': 'ok', 'data': {'ingress_port': 63001, 'ingress': True, 'ingress_panel': False}}
        self.assertEqual(module.supervisor_port(good), 63001)
        self.assertEqual(module.supervisor_port({'result': 'ok', 'data': dict(good['data'], ingress_panel=True)}), 63001)
        for field in ('ingress', 'panel_admin'):
            for value in (False, None):
                bad = {'result': 'ok', 'data': dict(good['data'], **{field: value})}
                with self.assertRaises(RuntimeError):
                    module.supervisor_port(bad)
        for value in (None, True, '63001', 63001.5, 0, 65501):
            with self.assertRaises(RuntimeError):
                module.supervisor_port({'result': 'ok', 'data': dict(good['data'], ingress_port=value)})
        for value in (None, {}, {'result': 'error', 'data': good['data']}, {'result':'ok','data':None}):
            with self.assertRaises(RuntimeError):
                module.supervisor_port(value)
        with tempfile.TemporaryDirectory() as tmp:
            db, startup = module.prepare({}, 63001, Path(tmp)/'data', Path(tmp)/'runtime')
            self.assertTrue(db.parent.is_dir())
            self.assertNotIn('initial_admin_file', json.loads(startup.read_text()))

    def test_yunohost_config_and_url_change_preserve_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            install, data = root / 'install', root / 'data'
            install.mkdir(); data.mkdir()
            args = [sys.executable, str(ROOT / 'packaging/yunohost/conf/configure.py'),
                    '--app', 'headscale_gateway__2', '--install-dir', str(install),
                    '--data-dir', str(data), '--port', '19091', '--lan-port', '18443',
                    '--proxy-include', str(root/'nginx/proxy-auth.inc')]
            subprocess.run(args + ['--path', '/gateway', '--admin', 'admin'],
                           input=b'a-password-123456', check=True, env=dict(os.environ, PYTHONPATH=str(ROOT/'packaging/common')))
            first = json.loads((install / 'startup.json').read_text())
            proof = first['proxy_auth']['shared_secret']
            self.assertEqual(first['proxy_auth']['unix_group'], 'admins')
            self.assertEqual(first['node_name'], 'headscale-gateway-2')
            include = root/'nginx/proxy-auth.inc'
            self.assertIn(proof, include.read_text())
            self.assertEqual(include.stat().st_mode & 0o777, 0o600)
            marker = data / 'gateway/master.key'; marker.write_bytes(b'key')
            Path(first['initial_admin_file']).unlink()
            subprocess.run(args + ['--path', '/new-gateway'], check=True, env=dict(os.environ, PYTHONPATH=str(ROOT/'packaging/common')))
            changed = json.loads((install / 'startup.json').read_text())
            self.assertEqual(changed['base_path'], '/new-gateway')
            self.assertEqual(changed['node_name'], first['node_name'])
            self.assertEqual(marker.read_bytes(), b'key')
            self.assertNotIn('initial_admin_file', changed)
            self.assertEqual(changed['proxy_auth']['shared_secret'], proof)

    def test_yunohost_sso_without_gateway_password(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            subprocess.run([sys.executable, str(ROOT/'packaging/yunohost/conf/configure.py'),
                '--app','headscale_gateway','--install-dir',str(root/'install'),
                '--data-dir',str(root/'data'),'--port','19091','--lan-port','18443',
                '--path','/','--proxy-include',str(root/'nginx/private.inc')],
                check=True,env=dict(os.environ,PYTHONPATH=str(ROOT/'packaging/common')))
            startup=json.loads((root/'install/startup.json').read_text())
            self.assertNotIn('initial_admin_file',startup)
            self.assertEqual(startup['base_path'],'')
            self.assertEqual(len(startup['proxy_auth']['shared_secret']),64)


if __name__ == '__main__':
    unittest.main()
