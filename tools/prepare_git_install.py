#!/usr/bin/env python3
"""Prepara el repositorio para instalar y actualizar desde Git privado, sin ZIP."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import tempfile

import package_platforms

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = 'git-install-manifest.json'
TREES = ('addons', 'yunohost')


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def files_under(directory):
    for path in sorted(directory.rglob('*')):
        if path.is_file() and not path.is_symlink() and not any(
                part in ('__pycache__', '.pytest_cache') for part in path.parts):
            yield path


def input_hashes(root):
    files = [p for p in (root / 'src').glob('*.go') if not p.name.endswith('_test.go')]
    for name in ('src/internal', 'packaging/homeassistant', 'packaging/yunohost',
                 'packaging/common', 'src/third_party/go', 'src/third_party/go-toolchain'):
        files.extend(p for p in files_under(root / name) if not p.name.endswith(('_test.go', '.test.cjs', '.pyc')))
    files.extend(root / name for name in ('VERSION', 'src/go.mod', 'src/go.sum', 'src/VERSION', 'LICENSE', 'NOTICE',
                 'src/third_party/inventory.json', 'tools/package_platforms.py', 'tools/prepare_git_install.py'))
    def source_digest(path):
        data = path.read_bytes()
        # El repositorio normaliza el texto mediante .gitattributes.
        if b'\0' not in data:
            data = data.replace(b'\r\n', b'\n')
        return hashlib.sha256(data).hexdigest()
    return {str(p.relative_to(root)): source_digest(p) for p in sorted(files)}


def output_hashes(root):
    return {str(p.relative_to(root)): digest(p)
            for tree in TREES for p in files_under(root / tree)}


def check(root, sources=True):
    path = root / MANIFEST
    if not path.is_file():
        raise ValueError('Missing git-install-manifest.json; run python3 tools/prepare_git_install.py')
    manifest = json.loads(path.read_text())
    if manifest.get('format') != 1 or not manifest.get('outputs'):
        raise ValueError('Invalid Git installation manifest')
    actual = output_hashes(root)
    if actual != manifest['outputs']:
        changed = sorted(k for k in actual.keys() | manifest['outputs'].keys()
                         if actual.get(k) != manifest['outputs'].get(k))
        raise ValueError('Generated files missing or modified: ' + ', '.join(changed[:8]))
    if sources and (manifest['inputs'] != input_hashes(root) or
                    manifest['version'] != (root / 'VERSION').read_text().strip()):
        raise ValueError('Installable folders are stale; run python3 tools/prepare_git_install.py')
    return manifest


def prepare(root):
    if (root / MANIFEST).exists():
        # Do not overwrite manual edits in generated directories.
        check(root, sources=False)
    elif any((root / name).exists() for name in TREES):
        raise ValueError('Refusing to replace existing unmanaged addons/ or yunohost/')
    before = input_hashes(root)
    with tempfile.TemporaryDirectory(prefix='hgw-git-install-') as temporary:
        stage = Path(temporary)
        package_platforms.export(stage / 'export', archives=False)
        shutil.copytree(stage / 'export/homeassistant/headscale_gateway', stage / 'addons/headscale_gateway')
        shutil.copytree(stage / 'export/yunohost', stage / 'yunohost')
        for name in TREES:
            # Los avisos contienen módulos Go con @version; deben quedar fuera de ./... .
            (stage / name / 'go.mod').write_text('module headscale-gateway-install-' + name + '\n\ngo 1.27.4\n')
        after = input_hashes(root)
        if before != after:
            raise ValueError('Sources changed during build; no installable files were replaced')
        manifest = {'format': 1, 'version': (root / 'VERSION').read_text().strip(),
                    'inputs': after, 'outputs': output_hashes(stage)}
        # Conserva los árboles generados anteriores hasta terminar la sustitución.
        backup = stage / 'previous'
        backup.mkdir()
        installed = []
        try:
            for name in TREES:
                if (root / name).exists():
                    shutil.move(str(root / name), backup / name)
                installed.append(name)
                shutil.copytree(stage / name, root / name)
            pending = root / (MANIFEST + '.next')
            pending.write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
            pending.replace(root / MANIFEST)
        except BaseException:
            for name in reversed(installed):
                if (root / name).exists():
                    shutil.rmtree(root / name)
                if (backup / name).exists():
                    shutil.move(str(backup / name), root / name)
            raise
    check(root)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='Comprueba los paquetes incluidos en Git sin compilar')
    args = parser.parse_args()
    try:
        if args.check:
            check(ROOT)
        else:
            prepare(ROOT)
    except (ValueError, OSError) as error:
        parser.exit(1, str(error) + '\n')
    print('OK: addons/headscale_gateway/ y yunohost/ coinciden con las fuentes comunes')


if __name__ == '__main__':
    main()
