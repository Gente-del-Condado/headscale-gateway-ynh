"""Contrato del árbol limpio: fuentes Android completas y cachés excluidas."""
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
from check_repository import check
from repository_archive import source_files


class CleanLayoutTest(unittest.TestCase):
    def test_translation_check_leaves_generated_files_untouched(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            tools = root / 'tools'
            tools.mkdir()
            for script in ('extract_i18n.py', 'check_i18n.py'):
                shutil.copyfile(ROOT / 'tools' / script, tools / script)
            generate = [sys.executable, '-B', str(tools / 'extract_i18n.py')]
            check_catalog = [sys.executable, '-B', str(tools / 'check_i18n.py')]
            subprocess.run(generate, check=True, capture_output=True)
            locale = root / 'src/locale'
            english = (locale / 'es.po').read_text().replace('Language: es', 'Language: en')
            (locale / 'en.po').write_text(english)
            subprocess.run(generate, check=True, capture_output=True)

            def timestamps():
                return {p: p.stat().st_mtime_ns for p in (root / 'src').rglob('*') if p.is_file()}

            before = timestamps()
            subprocess.run(check_catalog, check=True, capture_output=True)
            self.assertEqual(before, timestamps())

            stale = root / 'src/desktop/locales/en.json'
            stale.write_text('{"stale":true}\n')
            missing = root / 'src/internal/i18n/locales/es.json'
            missing.unlink()
            before = timestamps()
            result = subprocess.run(check_catalog, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            for path in (stale, missing):
                self.assertIn(str(path.relative_to(root)), result.stderr)
            self.assertEqual(stale.read_text(), '{"stale":true}\n')
            self.assertEqual(before, timestamps())

    def test_canonical_paths_and_licenses(self):
        check()

    def test_archive_contains_native_sources_and_no_build_results(self):
        names = {p.relative_to(ROOT).as_posix() for p in source_files()}
        required = {'src/main.go', 'src/go.mod', 'android/build.gradle',
                    'android/src/main/jni/Android.mk',
                    'android/src/main/jni/tunnel.c',
                    'android/src/androidTest/AndroidManifest.xml',
                    'src/third_party/inventory.json',
                    'docs/MIGRACION_REPOSITORIO.md'}
        self.assertFalse(required - names)
        self.assertFalse(any(n.startswith(('builds/', 'bin/', 'android/build/')) for n in names))
        self.assertFalse(any('/node_modules/' in n or '/.gradle/' in n for n in names))
        self.assertFalse(any(n.endswith('.apk') for n in names))
        self.assertIn('addons/headscale_gateway/source.tar.gz', names)
        self.assertIn('yunohost/payload/headscale-gateway-linux-arm64', names)


if __name__ == '__main__':
    unittest.main()
