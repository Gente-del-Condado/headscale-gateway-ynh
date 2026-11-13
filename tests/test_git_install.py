"""Private Git installations must ship a complete, current payload."""
import json
import hashlib
import tarfile
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
import prepare_git_install as git_install


class GitInstallTest(unittest.TestCase):
    def test_public_server_sources_match_the_manifest_and_both_platforms(self):
        source = ROOT / 'yunohost/payload/source.tar.gz'
        self.assertEqual(source.read_bytes(), (ROOT / 'addons/headscale_gateway/source.tar.gz').read_bytes())
        manifest = json.loads((ROOT / 'yunohost/build-manifest.json').read_text())
        with tarfile.open(source, 'r:gz') as archive:
            hashes = {member.name: hashlib.sha256(archive.extractfile(member).read()).hexdigest()
                      for member in archive.getmembers()}
            self.assertEqual(hashes, manifest['sources'])
            self.assertEqual(archive.extractfile('LICENSE').read(), (ROOT / 'LICENSE').read_bytes())
            self.assertEqual(archive.extractfile('NOTICE').read(), (ROOT / 'NOTICE').read_bytes())
            self.assertIn('SOURCE.md', hashes)
            self.assertIn('main.go', hashes)

    def test_committed_packages_match_sources(self):
        manifest = git_install.check(ROOT)
        self.assertEqual(manifest['version'], (ROOT / 'VERSION').read_text().strip())
        for name in ('Dockerfile', 'source.tar.gz', 'hgw_bootstrap.py'):
            self.assertIn('addons/headscale_gateway/' + name, manifest['outputs'])
        for arch in ('amd64', 'arm64'):
            self.assertIn('yunohost/payload/headscale-gateway-linux-' + arch, manifest['outputs'])

    def test_tampering_and_missing_payload_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            binary = root / 'yunohost/payload/program'
            binary.parent.mkdir(parents=True)
            binary.write_bytes(b'checked binary')
            manifest = {'format': 1, 'version': '1.2.0', 'outputs': git_install.output_hashes(root)}
            (root / git_install.MANIFEST).write_text(json.dumps(manifest))
            git_install.check(root, sources=False)
            binary.write_bytes(b'wrong binary')
            with self.assertRaisesRegex(ValueError, 'missing or modified'):
                git_install.check(root, sources=False)
            binary.unlink()
            with self.assertRaisesRegex(ValueError, 'missing or modified'):
                git_install.check(root, sources=False)

    def test_unmanaged_directories_are_not_replaced(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            marker = root / 'yunohost/personal-file'
            marker.parent.mkdir()
            marker.write_text('keep me')
            with self.assertRaisesRegex(ValueError, 'unmanaged'):
                git_install.prepare(root)
            self.assertEqual(marker.read_text(), 'keep me')

    def test_changed_sources_require_regeneration(self):
        with mock.patch.object(git_install, 'input_hashes', return_value={'changed.go': 'new hash'}):
            with self.assertRaisesRegex(ValueError, 'stale'):
                git_install.check(ROOT)


if __name__ == '__main__':
    unittest.main()
