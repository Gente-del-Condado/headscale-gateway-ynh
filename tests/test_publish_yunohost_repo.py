"""La distribución pública contiene solo el paquete íntegro del commit aprobado."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
import publish_yunohost_repo as publisher


class PublishYunoHostTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / 'source'
        self.destination = self.root / 'destination'
        for repository in (self.source, self.destination):
            repository.mkdir()
            self.git(repository, 'init', '--initial-branch=main')
            self.git(repository, 'config', 'user.name', 'Pruebas')
            self.git(repository, 'config', 'user.email', 'tests@example.invalid')
        self.package = self.source / 'yunohost'
        for relative in ('LICENSE', 'README.md', 'tests.toml', 'go.mod', 'doc/ADMIN.md', 'doc/logo.png',
                         'scripts/install', 'scripts/upgrade', 'scripts/backup',
                         'scripts/restore', 'conf/hgw_bootstrap.py',
                         'payload/notices/LICENSE', 'payload/notices/NOTICE',
                         'payload/notices/third_party/inventory.json'):
            self.write(relative, 'Contenido de prueba\n')
        sums = {}
        for name in publisher.BINARIES:
            self.write('payload/' + name, '#!/bin/sh\nexit 0\n', executable=True)
            sums[name] = hashlib.sha256((self.package / 'payload' / name).read_bytes()).hexdigest()
        self.write('scripts/install', '#!/bin/bash\nexit 0\n', executable=True)
        self.write('payload/SHA256SUMS', ''.join(f'{digest}  {name}\n' for name, digest in sums.items()))
        self.write('manifest.toml', 'id = "headscale_gateway"\nversion = "1.4.6~ynh1"\n')
        sources = b'package main\nfunc main() {}\n'
        with tarfile.open(self.package / 'payload/source.tar.gz', 'w:gz') as archive:
            member = tarfile.TarInfo('main.go')
            member.size = len(sources)
            archive.addfile(member, io.BytesIO(sources))
        self.write('build-manifest.json', json.dumps({'version': '1.4.6', 'commercial': False,
                   'binaries': sums, 'sources': {'main.go': hashlib.sha256(sources).hexdigest()}}))
        (self.source / 'VERSION').write_text('1.4.6\n')
        (self.source / 'private-source.go').write_text('No publicar\n')
        self.commit()

    def git(self, repository, *args):
        return subprocess.check_output(['git', '-C', str(repository), *args], stderr=subprocess.STDOUT)

    def write(self, relative, content, executable=False):
        path = self.package / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        if executable:
            path.chmod(0o755)

    def commit(self):
        self.git(self.source, 'add', '.')
        self.git(self.source, 'commit', '-m', 'Paquete de prueba')

    def test_only_committed_package_is_exported_and_repeated_publication_is_identical(self):
        self.write('README.md', 'Cambio sin aprobar\n')
        (self.destination / '.github').mkdir()
        marker = self.destination / '.github' / 'keep.yml'
        marker.write_text('Conservar\n')
        stale = self.destination / 'payload' / 'obsolete'
        stale.parent.mkdir()
        stale.write_text('Eliminar\n')
        publisher.publish(self.source, self.destination)
        self.assertFalse((self.destination / 'private-source.go').exists())
        self.assertFalse((self.destination / 'yunohost').exists())
        self.assertFalse(stale.exists())
        self.assertEqual((self.destination / 'README.md').read_text(), 'Contenido de prueba\n')
        self.assertEqual(marker.read_text(), 'Conservar\n')
        self.assertTrue((self.destination / 'scripts/install').stat().st_mode & 0o111)
        self.git(self.destination, 'add', '.')
        self.git(self.destination, 'commit', '-m', 'Primera publicación')
        publisher.publish(self.source, self.destination)
        self.assertEqual(self.git(self.destination, 'status', '--porcelain'), b'')

    def test_corrupt_payload_is_rejected_before_changing_destination(self):
        self.write('payload/' + publisher.BINARIES[0], 'Alterado\n', executable=True)
        self.commit()
        marker = self.destination / 'README.md'
        marker.write_text('Versión publicada\n')
        with self.assertRaisesRegex(ValueError, 'suma'):
            publisher.publish(self.source, self.destination)
        self.assertEqual(marker.read_text(), 'Versión publicada\n')

    def test_corrupt_sources_are_rejected_before_changing_destination(self):
        with tarfile.open(self.package / 'payload/source.tar.gz', 'w:gz') as archive:
            content = b'fuentes de otra compilacion'
            member = tarfile.TarInfo('main.go')
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
        self.commit()
        marker = self.destination / 'README.md'
        marker.write_text('Versión publicada\n')
        with self.assertRaisesRegex(ValueError, 'fuentes'):
            publisher.publish(self.source, self.destination)
        self.assertEqual(marker.read_text(), 'Versión publicada\n')

    def test_version_mismatch_and_missing_execution_permission_are_rejected(self):
        self.write('manifest.toml', 'id = "headscale_gateway"\nversion = "0.0.0~ynh1"\n')
        self.commit()
        with self.assertRaisesRegex(ValueError, 'versión'):
            publisher.publish(self.source, self.destination)
        self.write('manifest.toml', 'id = "headscale_gateway"\nversion = "1.4.6~ynh1"\n')
        (self.package / 'payload' / publisher.BINARIES[0]).chmod(0o644)
        self.commit()
        with self.assertRaisesRegex(ValueError, 'ejecución'):
            publisher.publish(self.source, self.destination)

    def test_links_and_unknown_paths_cannot_escape_the_package(self):
        for name, kind in (('../outside', tarfile.REGTYPE), ('src/private.go', tarfile.REGTYPE),
                           ('payload/link', tarfile.SYMTYPE), ('payload/hard', tarfile.LNKTYPE)):
            with self.subTest(name=name):
                data = io.BytesIO()
                with tarfile.open(fileobj=data, mode='w') as archive:
                    member = tarfile.TarInfo(name)
                    member.type = kind
                    member.linkname = '/etc/passwd'
                    archive.addfile(member)
                with self.assertRaisesRegex(ValueError, 'no publicable'):
                    publisher.validate_archive(data.getvalue(), self.root / 'stage')

    def test_destination_must_be_separate_and_cannot_redirect_managed_files(self):
        with self.assertRaisesRegex(ValueError, 'separado'):
            publisher.publish(self.source, self.source)
        subdirectory = self.destination / 'nested'
        subdirectory.mkdir()
        with self.assertRaisesRegex(ValueError, 'raíz'):
            publisher.publish(self.source, subdirectory)
        outside = self.root / 'outside'
        outside.write_text('Conservar\n')
        (self.destination / 'README.md').symlink_to(outside)
        with self.assertRaisesRegex(ValueError, 'enlace'):
            publisher.publish(self.source, self.destination)
        self.assertEqual(outside.read_text(), 'Conservar\n')


if __name__ == '__main__':
    unittest.main()
