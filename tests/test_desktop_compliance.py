"""Una entrega de escritorio debe conservar runtime, fuentes y avisos completos."""
import json
from pathlib import Path
import struct
import sys
import tarfile
import tempfile
import unittest
from unittest import mock
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
import desktop_compliance as compliance
import package_release


class DesktopComplianceTest(unittest.TestCase):
    def test_committed_sources_track_the_electron_version_in_npm(self):
        root = Path(__file__).resolve().parents[1]
        lock = json.loads((root / 'packaging/desktop/electron-sources.lock.json').read_text())
        package = json.loads((root / 'src/desktop/package.json').read_text())
        npm_lock = json.loads((root / 'src/desktop/package-lock.json').read_text())
        self.assertEqual(lock['electron_version'], package['devDependencies']['electron'])
        self.assertEqual(lock['electron_version'], npm_lock['packages']['node_modules/electron']['version'])
        self.assertEqual(set(lock['runtimes']), {'linux', 'win32'})
        self.assertIn('chromium-' + lock['chromium_version'] + '.tar.xz',
                      [record['name'] for record in lock['sources']])

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.runtime = self.root / 'bin/linux/Gateway-Tray'
        self.runtime.mkdir(parents=True)
        self.cache = self.root / 'cache'
        self.cache.mkdir()
        self.lock = {'format': 1, 'electron_version': '45.0.0', 'sources': [], 'runtimes': {}}
        archive = self.cache / 'electron.zip'
        with zipfile.ZipFile(archive, 'w') as z:
            for name, data in {'electron': b'upstream binary', 'version': b'45.0.0',
                               'libffmpeg.so': b'upstream ffmpeg', 'LICENSE': b'MIT upstream',
                               'LICENSES.chromium.html': b'LGPL and MPL notices'}.items():
                z.writestr(name, data)
                (self.runtime / ('Gateway-Tray' if name == 'electron' else name)).write_bytes(data)
        self.lock['runtimes']['linux'] = self.record(archive)
        source = self.cache / 'source.tar.xz'
        source.write_bytes(b'archived corresponding sources')
        self.lock['sources'] = [self.record(source)]
        for relative in ('LICENSE', 'NOTICE', 'VERSION', 'tools/desktop_compliance.py',
                         'packaging/desktop/README.md', 'packaging/desktop/electron-sources.lock.json',
                         'src/desktop/package.json', 'src/desktop/package-lock.json',
                         'src/desktop/pack.cjs', 'src/desktop/main.cjs'):
            p = self.root / relative
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('source ' + relative)
        self.patch = mock.patch.object(compliance, 'ROOT', self.root)
        self.patch.start()
        self.addCleanup(self.patch.stop)

    def record(self, path):
        return {'name': path.name, 'url': 'https://example.invalid/' + path.name,
                'sha256': compliance.digest(path), 'size': path.stat().st_size}

    def prepare(self):
        compliance.prepare(self.runtime, 'linux', self.cache, self.lock)

    def test_complete_package_keeps_upstream_notices_and_own_sources(self):
        self.prepare()
        result = compliance.verify(self.runtime, 'linux', self.lock)
        self.assertEqual(result['lock'], self.lock)
        self.assertEqual((self.runtime / 'legal/LICENSE').read_bytes(), b'MIT upstream')
        self.assertEqual((self.runtime / 'legal/application-source/LICENSE').read_bytes(),
                         (self.root / 'LICENSE').read_bytes())
        self.assertEqual((self.runtime / 'legal/sources/source.tar.xz').read_bytes(),
                         (self.cache / 'source.tar.xz').read_bytes())

    def test_wrong_runtime_or_corrupt_cache_cannot_replace_existing_legal_files(self):
        self.prepare()
        original = (self.runtime / 'legal/manifest.json').read_bytes()
        (self.runtime / 'libffmpeg.so').write_bytes(b'modified')
        with self.assertRaisesRegex(ValueError, 'modificado'):
            self.prepare()
        self.assertEqual((self.runtime / 'legal/manifest.json').read_bytes(), original)
        (self.runtime / 'libffmpeg.so').write_bytes(b'upstream ffmpeg')
        (self.cache / 'source.tar.xz').write_bytes(b'wrong version of source')
        with self.assertRaisesRegex(ValueError, 'alterado'):
            self.prepare()
        self.assertEqual((self.runtime / 'legal/manifest.json').read_bytes(), original)

    def test_missing_sources_extra_files_and_runtime_changes_block_publication(self):
        self.prepare()
        source = self.runtime / 'legal/sources/source.tar.xz'
        source.unlink()
        with self.assertRaisesRegex(ValueError, 'han cambiado'):
            compliance.verify(self.runtime, 'linux', self.lock)
        self.prepare()
        (self.runtime / 'private-profile').write_bytes(b'not part of release')
        with self.assertRaisesRegex(ValueError, 'han cambiado'):
            compliance.verify(self.runtime, 'linux', self.lock)
        (self.runtime / 'private-profile').unlink()
        (self.runtime / 'Gateway-Tray').write_bytes(b'other binary')
        with self.assertRaisesRegex(ValueError, 'han cambiado'):
            compliance.verify(self.runtime, 'linux', self.lock)

    def test_release_archive_physically_contains_corresponding_sources(self):
        self.prepare()
        output = self.root / 'release'
        output.mkdir()
        with mock.patch.object(package_release, 'ROOT', self.root), \
             mock.patch.object(package_release, 'verify_desktop',
                               side_effect=lambda p, platform: compliance.verify(p, platform, self.lock)):
            artifacts = package_release.desktop_archives(output)
        self.assertEqual(len(artifacts), 1)
        with tarfile.open(artifacts[0]) as archive:
            self.assertEqual(archive.extractfile('Gateway-Tray/legal/sources/source.tar.xz').read(),
                             (self.cache / 'source.tar.xz').read_bytes())
            self.assertEqual(archive.extractfile('Gateway-Tray/legal/LICENSE').read(), b'MIT upstream')

    def test_pe_branding_does_not_hide_code_changes(self):
        data = bytearray(300)
        data[:2] = b'MZ'
        struct.pack_into('<I', data, 0x3c, 64)
        data[64:68] = b'PE\0\0'
        struct.pack_into('<H', data, 70, 1)
        data[88:96] = b'.text\0\0\0'
        struct.pack_into('<II', data, 104, 4, 240)
        data[240:244] = b'code'
        branded = data.copy()
        branded[280:284] = b'icon'
        self.assertEqual(compliance.pe_code(data), compliance.pe_code(branded))
        branded[240:244] = b'evil'
        self.assertNotEqual(compliance.pe_code(data), compliance.pe_code(branded))


if __name__ == '__main__':
    unittest.main()
