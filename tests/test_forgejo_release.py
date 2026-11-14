import hashlib
import http.client
import http.server
import importlib.util
import json
import os
import subprocess
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('forgejo_release', ROOT/'tools/publish_forgejo_release.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ReleaseTest(unittest.TestCase):
    def test_streaming_upload_and_download_verify_actual_bytes(self):
        payload = b'fixture-' * 150000
        observed = []
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_POST(self):
                body = self.rfile.read(int(self.headers['Content-Length']))
                observed.append((self.path, self.headers['Authorization'], body))
                answer = json.dumps({'name': 'artifact.zip', 'size': len(payload),
                    'type': 'attachment', 'uuid': '00112233-4455-6677-8899-aabbccddeeff',
                    'browser_download_url': 'https://gentedelcondado.es/forgejo/attachments/00112233-4455-6677-8899-aabbccddeeff'}).encode()
                self.send_response(201); self.send_header('Content-Length', str(len(answer)))
                self.end_headers(); self.wfile.write(answer)
            def do_GET(self):
                observed.append((self.path, self.headers['Authorization']))
                self.send_response(200); self.send_header('Content-Length', str(len(payload)))
                self.end_headers(); self.wfile.write(payload)
        server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        original_connection = http.client.HTTPConnection
        def connect(host, port, timeout):
            self.assertEqual((host, port), ('127.0.0.1', 6000))
            return original_connection('127.0.0.1', server.server_port, timeout=timeout)
        try:
            with tempfile.TemporaryDirectory() as temporary:
                file = Path(temporary)/'artifact.zip'; file.write_bytes(payload)
                expected = {'name': file.name, 'bytes': len(payload),
                            'sha256': hashlib.sha256(payload).hexdigest()}
                with patch.object(module.http.client, 'HTTPConnection', side_effect=connect):
                    client = module.Forgejo('fixture-token')
                    remote = client.upload(4, file)
                    client.verify(remote, expected)
                    with self.assertRaises(ValueError):
                        client.verify(remote, dict(expected, sha256='0'*64))
                self.assertEqual(observed[0][0], module.API+'/releases/4/assets')
                self.assertIn(payload, observed[0][2])
                self.assertTrue(observed[0][2].endswith(b'--\r\n'))
                self.assertEqual(observed[1][0], '/attachments/00112233-4455-6677-8899-aabbccddeeff')
                self.assertTrue(all(item[1] == 'Bearer fixture-token' for item in observed))
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def test_manifest_rejects_links_tampering_and_extra_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            file = folder/'artifact.zip'
            file.write_bytes(b'fixture')
            asset = {'name': file.name, 'bytes': file.stat().st_size,
                     'sha256': hashlib.sha256(file.read_bytes()).hexdigest()}
            data = {'tag': 'v1.4.7', 'source_commit': 'a'*40,
                    'body': 'Notas revisadas', 'assets': [asset]}
            manifest = folder/'release-assets.json'
            manifest.write_text(json.dumps(data))
            # La prueba usa la propiedad del usuario local; producción exige
            # root también en cada antecesor de la carpeta.
            with patch.object(module, 'private_tree'):
                self.assertEqual(module.load_manifest(folder, 'v1.4.7', 'a'*40, os.getuid()), data)
                file.write_bytes(b'changed')
                with self.assertRaises(ValueError):
                    module.load_manifest(folder, 'v1.4.7', 'a'*40, os.getuid())
                file.unlink(); file.symlink_to(manifest)
                with self.assertRaises(ValueError):
                    module.load_manifest(folder, 'v1.4.7', 'a'*40, os.getuid())
                file.unlink(); file.write_bytes(b'fixture')
                (folder/'unexpected.txt').write_text('extra')
                with self.assertRaises(ValueError):
                    module.load_manifest(folder, 'v1.4.7', 'a'*40, os.getuid())
                with self.assertRaises(ValueError):
                    module.load_manifest(folder, 'v1.4.7', 'b'*40, os.getuid())

    def test_publish_only_after_every_remote_hash_is_verified(self):
        asset = {'name': 'artifact.zip', 'bytes': 7, 'sha256': 'a'*64}
        data = {'assets': [asset], 'body': 'Notas'}
        events = []
        class Client:
            def request(self, method, path, data=None, missing=False):
                events.append((method, path))
                if path.endswith('/branches/main'): return {'commit': {'id': 'a'*40}}
                if path == module.API+'/tags/v1.4.7': return None if missing else {'commit': {'sha': 'a'*40}}
                if missing: return None
                if path.endswith('/assets'): return []
                if method == 'POST': return {'id': 4, 'draft': True}
                if method == 'GET': return {'id': 4, 'draft': False, 'tag_name': 'v1.4.7'}
            def upload(self, release_id, file):
                events.append(('upload', file.name)); return {'name': file.name}
            def verify(self, remote, expected):
                events.append(('verify', expected['name']))
        with patch.object(module, 'load_manifest', return_value=data):
            module.publish('/fixture', 'v1.4.7', 'a'*40, Client())
            self.assertLess(events.index(('verify', 'artifact.zip')),
                            events.index(('PATCH', module.API+'/releases/4')))
            events.clear()
            with patch.object(Client, 'verify', side_effect=ValueError('hash incorrecto')):
                with self.assertRaises(ValueError):
                    module.publish('/fixture', 'v1.4.7', 'a'*40, Client())
                self.assertFalse(any(method == 'PATCH' for method, _ in events))
            events.clear()
            module.publish('/fixture', 'v1.4.7', 'a'*40, Client(), draft_only=True)
            self.assertFalse(any(method == 'PATCH' for method, _ in events))

    def test_existing_attachments_are_listed_once_without_pagination(self):
        asset = {'name': 'artifact.zip', 'bytes': 7, 'sha256': 'a'*64}
        events = []
        class Client:
            def request(self, method, path, data=None, missing=False):
                events.append((method, path))
                if path.endswith('/branches/main'): return {'commit': {'id': 'a'*40}}
                if path == module.API+'/tags/v1.4.7': return {'commit': {'sha': 'a'*40}}
                if path.endswith('/assets'): return [{'name': 'artifact.zip'}]
                if path.endswith('/releases/tags/v1.4.7'): return {'id': 4, 'draft': True}
                if method == 'GET': return {'id': 4, 'draft': False, 'tag_name': 'v1.4.7'}
            def upload(self, *args):
                raise AssertionError('No debe subir un adjunto ya existente')
            def verify(self, remote, expected):
                events.append(('verify', expected['name']))
        with patch.object(module, 'load_manifest', return_value={'assets': [asset], 'body': 'Notas'}):
            module.publish('/fixture', 'v1.4.7', 'a'*40, Client())
        self.assertEqual(events.count(('GET', module.API+'/releases/4/assets')), 1)
        self.assertIn(('verify', 'artifact.zip'), events)
        self.assertFalse(any('?page=' in path or '?limit=' in path for _, path in events))

    def test_retry_preserves_tag_and_rejects_product_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            repo = Path(temporary)
            def git(*args):
                return subprocess.check_output(['git', '-C', temporary, *args], text=True,
                                               stderr=subprocess.DEVNULL).strip()
            git('init', '-q'); git('config', 'user.name', 'Fixture')
            git('config', 'user.email', 'fixture@example.invalid')
            (repo/'VERSION').write_text('1.4.7')
            git('add', '.'); git('commit', '-qm', 'Producto'); git('tag', 'v1.4.7')
            original = git('rev-parse', 'HEAD')
            (repo/'tools').mkdir(); (repo/'tools/publish_forgejo_release.py').write_text('# Arreglo')
            git('add', '.'); git('commit', '-qm', 'Publicador')
            head = git('rev-parse', 'HEAD')
            self.assertEqual(module.resolve_release_commit('v1.4.7', head, repository=repo), original)
            self.assertEqual(git('rev-parse', 'v1.4.7'), original)
            with self.assertRaises(ValueError):
                module.resolve_release_commit('v1.4.8', head, repository=repo)
            self.assertEqual(module.resolve_release_commit('v1.4.8', head, True, repo), head)
            (repo/'src').mkdir(); (repo/'src/main.go').write_text('// Motor distinto')
            git('add', '.'); git('commit', '-qm', 'Motor')
            with self.assertRaises(ValueError):
                module.resolve_release_commit('v1.4.7', git('rev-parse', 'HEAD'), repository=repo)
            git('checkout', '--orphan', 'unrelated'); git('commit', '-qm', 'Otra historia')
            with self.assertRaises(ValueError):
                module.resolve_release_commit('v1.4.7', git('rev-parse', 'HEAD'), repository=repo)

    def test_remote_metadata_cannot_send_token_to_another_host(self):
        expected = {'name': 'artifact.zip', 'bytes': 7,
                    'sha256': hashlib.sha256(b'fixture').hexdigest()}
        remote = {'name': 'artifact.zip', 'size': 7, 'type': 'attachment',
                  'uuid': '00112233-4455-6677-8899-aabbccddeeff',
                  'browser_download_url': 'https://example.invalid/collect-token'}
        with patch.object(module.http.client, 'HTTPConnection') as connect:
            response = connect.return_value.getresponse.return_value
            response.status = 200
            response.read.side_effect = [b'fixture', b'']
            module.Forgejo('fixture-token').verify(remote, expected)
            connect.assert_called_once_with('127.0.0.1', 6000, timeout=300)
            connect.return_value.request.assert_called_once_with('GET',
                '/attachments/00112233-4455-6677-8899-aabbccddeeff',
                headers={'Authorization': 'Bearer fixture-token'})
        with patch.object(module.http.client, 'HTTPConnection') as connect:
            with self.assertRaises(ValueError):
                module.Forgejo('fixture-token').verify(dict(remote, uuid='../../collect'), expected)
            connect.assert_not_called()


if __name__ == '__main__':
    unittest.main()
