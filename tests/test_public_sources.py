"""Los fuentes públicos no deben arrastrar el historial del repositorio privado."""
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
import publish_public_sources


class PublicSourcesTest(unittest.TestCase):
    def test_orphan_history_and_immutable_version_tag(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source, remote, package = root / 'source', root / 'remote.git', root / 'package'
            source.mkdir()
            def git(folder, *args):
                return subprocess.check_output(['git', '-C', str(folder), *args], stderr=subprocess.DEVNULL, text=True).strip()
            subprocess.run(['git', 'init', '--bare', str(remote)], check=True, capture_output=True)
            subprocess.run(['git', 'clone', str(remote), str(package)], check=True, capture_output=True)
            git(package, 'config', 'core.sshCommand', 'ssh -o BatchMode=yes')
            version = source / 'VERSION'
            version.write_text('1.4.7\n')
            program = source / 'program.go'
            program.write_text('package main\n')
            (source / 'addons').mkdir()
            excluded = source / 'addons' / 'private-installer'
            excluded.write_text('not part of the public source snapshot')
            with patch.object(publish_public_sources, 'ROOT', source), patch.object(
                    publish_public_sources, 'source_files', return_value=[version, program, excluded]):
                publish_public_sources.publish(package, tag=True)
                original = git(package, 'ls-remote', '--tags', 'origin', 'refs/tags/source-v1.4.7').split()[0]
                program.write_text('package updated\n')
                publish_public_sources.publish(package, tag=True)
                current = git(package, 'ls-remote', '--tags', 'origin', 'refs/tags/source-v1.4.7').split()[0]
            self.assertEqual(original, current)
            git(package, 'fetch', 'origin', 'upstream-source')
            self.assertEqual(git(package, 'rev-list', '--count', 'FETCH_HEAD'), '2')
            names = git(package, 'ls-tree', '-r', '--name-only', 'FETCH_HEAD').splitlines()
            self.assertIn('program.go', names)
            self.assertIn('CONTENIDO.sha256', names)
            self.assertNotIn('addons/private-installer', names)
