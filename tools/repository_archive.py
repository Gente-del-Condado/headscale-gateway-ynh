#!/usr/bin/env python3
"""Exporta el repositorio limpio mediante una lista explícita de carpetas."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DIRECTORIES = {'src', 'android', 'tools', 'tests', 'docs', 'examples',
               'packaging', 'custom_components', 'addons', 'yunohost', '.forgejo', '.github'}
ROOT_FILES = {'README.md', 'CONTRIBUTING.md', 'LICENSE', 'NOTICE', 'VERSION',
              '.gitignore', '.gitattributes', 'hacs.json', 'git-install-manifest.json',
              'build.sh', 'build.ps1', 'AGENTS.md', 'mkdocs.yml', 'requirements-docs.txt'}
CACHE = {'node_modules', '__pycache__', '.pytest_cache', '.gradle', '.kotlin', '.cxx', '.git', '.cache', '.idea', '.vscode'}
RELEASE_FILES = {'headscale-gateway-debug.apk', 'headscale-gateway-debug.apk.sha256',
                 'headscale-gateway-release.apk', 'headscale-gateway-release.apk.sha256', 'VALIDACION.txt'}
PRIVATE_SUFFIXES = ('.sqlite', '.sqlite-wal', '.sqlite-shm', '.key', '.pfx', '.p12', '.jks', '.keystore', '.hgw', '.hgwcfg', '.log', '.license', '.pyc', '.next', '.bak')
SECRET = re.compile(rb'hskey-(?:auth|api)-[A-Za-z0-9_-]{20,}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----\s+[A-Za-z0-9+/=\r\n]{100,}-----END')


def source_files(include_releases=False):
    manifest = json.loads((ROOT / 'git-install-manifest.json').read_text())
    generated = set(manifest['outputs'])
    for p in sorted(ROOT.rglob('*')):
        rel = p.relative_to(ROOT)
        if not p.is_file():
            continue
        if rel.is_relative_to('android/build') or rel.is_relative_to('src/desktop/dist'):
            continue
        if any(part in CACHE for part in rel.parts):
            continue
        if p.is_symlink():
            raise ValueError('No se exportan enlaces simbólicos: ' + str(rel))
        if len(rel.parts) == 1:
            if p.name not in ROOT_FILES:
                continue
        elif rel.parts[0] == 'releases':
            if rel == Path('releases/README.md'):
                pass
            elif not include_releases or len(rel.parts) != 3 or rel.parts[1] != 'android' or p.name not in RELEASE_FILES:
                continue
        elif rel.parts[0] not in DIRECTORIES:
            continue
        if 'android/src/main/assets/licenses' in str(rel) or rel == Path('android/src/main/assets/avisos.txt'):
            continue
        if p.name in ('local.properties', '.env', 'master.key') or p.name.startswith('.env.') or p.name.endswith(PRIVATE_SUFFIXES):
            raise ValueError('Estado privado dentro de las fuentes: ' + str(rel))
        if rel.parts[0] in ('addons', 'yunohost') and str(rel) not in generated:
            raise ValueError('Archivo instalable ajeno al manifiesto: ' + str(rel))
        if p.suffix != '.apk' and SECRET.search(p.read_bytes()):
            raise ValueError('Posible credencial en: ' + str(rel))
        yield p


def export(destination, include_releases=False):
    from prepare_git_install import check
    check(ROOT)
    files = list(source_files(include_releases))
    for p in files:
        if p.suffix == '.apk':
            checksum = p.with_suffix('.apk.sha256')
            if not checksum.is_file() or checksum.read_text().split()[0] != hashlib.sha256(p.read_bytes()).hexdigest():
                raise ValueError('Falta una suma válida para la APK: ' + p.name)
    destination = destination.resolve()
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.exists():
        raise ValueError('El archivo ya existe: ' + str(destination))
    checks = []
    pending = destination.with_suffix(destination.suffix + '.next')
    try:
        with zipfile.ZipFile(pending, 'w', zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
            for p in files:
                rel = p.relative_to(ROOT).as_posix()
                archive.write(p, 'headscale-gateway/' + rel)
                checks.append(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + rel)
            archive.writestr('headscale-gateway/CONTENIDO.sha256', '\n'.join(checks) + '\n')
        with zipfile.ZipFile(pending) as archive:
            if archive.testzip() is not None:
                raise ValueError('La comprobación interna del ZIP ha fallado')
        pending.replace(destination)
    finally:
        pending.unlink(missing_ok=True)
    destination.with_suffix('.zip.sha256').write_text(hashlib.sha256(destination.read_bytes()).hexdigest() + '  ' + destination.name + '\n')
    print(f'ZIP verificado: {destination} ({len(files)} archivos)')
    return destination


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--include-releases', action='store_true', help='Incluye únicamente las entregas Android de releases/android/')
    args = parser.parse_args()
    export(args.output, args.include_releases)
