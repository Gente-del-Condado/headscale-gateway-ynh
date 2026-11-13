"""El manual se publica en su rama sin sustituir el paquete instalable."""
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
from publish_manual import publish


class PublishManualTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / 'manual'
        self.target = self.root / 'repository'
        self.source.mkdir()
        self.target.mkdir()
        self.git('init', '--initial-branch=gh-pages')
        for name in ('index.html', 'YUNOHOST.html', 'HOME_ASSISTANT.html', 'INTEGRACION_HA.html'):
            (self.source / name).write_text('<html>Manual</html>')

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.target), *args], stderr=subprocess.STDOUT)

    def test_publication_removes_obsolete_pages_and_is_idempotent(self):
        (self.target / 'obsolete.html').write_text('Antigua')
        publish(self.source, self.target)
        self.assertFalse((self.target / 'obsolete.html').exists())
        self.assertTrue((self.target / '.git').is_dir())
        self.assertTrue((self.target / '.nojekyll').is_file())
        self.git('add', '.')
        self.git('-c', 'user.name=Tests', '-c', 'user.email=tests@example.invalid', 'commit', '-m', 'Manual')
        publish(self.source, self.target)
        self.assertEqual(self.git('status', '--porcelain'), b'')

    def test_main_and_subdirectory_are_rejected_before_changing_files(self):
        marker = self.target / 'manifest.toml'
        marker.write_text('Paquete')
        self.git('symbolic-ref', 'HEAD', 'refs/heads/main')
        with self.assertRaisesRegex(ValueError, 'nunca main'):
            publish(self.source, self.target)
        self.assertEqual(marker.read_text(), 'Paquete')
        child = self.target / 'nested'
        child.mkdir()
        with self.assertRaisesRegex(ValueError, 'raíz'):
            publish(self.source, child)

    def test_missing_pages_links_and_git_metadata_are_not_published(self):
        index = self.source / 'index.html'
        index.unlink()
        with self.assertRaisesRegex(ValueError, 'Falta'):
            publish(self.source, self.target)
        index.symlink_to(self.source / 'YUNOHOST.html')
        with self.assertRaisesRegex(ValueError, 'enlaces'):
            publish(self.source, self.target)
        index.unlink()
        index.write_text('Manual')
        (self.source / '.git').mkdir()
        with self.assertRaisesRegex(ValueError, 'metadatos Git'):
            publish(self.source, self.target)


if __name__ == '__main__':
    unittest.main()
