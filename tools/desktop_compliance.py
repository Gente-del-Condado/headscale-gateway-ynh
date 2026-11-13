#!/usr/bin/env python3
"""Prepara y verifica fuentes, avisos y procedencia del escritorio distribuido."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import struct
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / 'packaging/desktop/electron-sources.lock.json'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def files(root):
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            raise ValueError('No se distribuyen enlaces en el paquete de escritorio: ' + str(path))
        if path.is_file():
            yield path


def acquire(record, cache):
    name = record['name']
    if Path(name).name != name or not record['url'].startswith('https://'):
        raise ValueError('Ruta u origen no válido en el manifiesto de fuentes.')
    target = cache / name
    if target.is_symlink():
        raise ValueError('La caché no puede contener enlaces.')
    if not target.exists():
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as stream:
            pending = Path(stream.name)
            try:
                with urllib.request.urlopen(record['url'], timeout=90) as response:
                    shutil.copyfileobj(response, stream)
                stream.close()
                if digest(pending) != record['sha256']:
                    raise ValueError('La descarga no coincide con la suma fijada: ' + name)
                pending.replace(target)
            finally:
                pending.unlink(missing_ok=True)
    if target.stat().st_size != record['size'] or digest(target) != record['sha256']:
        raise ValueError('Archivo de fuentes o runtime alterado: ' + name)
    return target


def pe_code(data):
    # rcedit cambia iconos/metadatos del PE; el código ejecutable debe conservarse.
    if data[:2] != b'MZ':
        raise ValueError('El ejecutable Windows no es PE.')
    pe = struct.unpack_from('<I', data, 0x3c)[0]
    if data[pe:pe + 4] != b'PE\0\0':
        raise ValueError('Cabecera PE no válida.')
    count = struct.unpack_from('<H', data, pe + 6)[0]
    offset = pe + 24 + struct.unpack_from('<H', data, pe + 20)[0]
    for index in range(count):
        section = offset + index * 40
        if data[section:section + 8].rstrip(b'\0') == b'.text':
            size, start = struct.unpack_from('<II', data, section + 16)
            if start + size > len(data):
                break
            return data[start:start + size]
    raise ValueError('No se encuentra la sección ejecutable de Electron.')


def check_runtime(runtime, archive, platform):
    binary = 'electron.exe' if platform == 'win32' else 'electron'
    renamed = 'Gateway-Tray.exe' if platform == 'win32' else 'Gateway-Tray'
    with zipfile.ZipFile(archive) as original:
        for member in original.infolist():
            if member.is_dir() or member.filename == 'resources/default_app.asar':
                continue
            relative = renamed if member.filename == binary else member.filename
            path = runtime / relative
            if not path.is_file() or path.is_symlink():
                raise ValueError('Falta un componente original de Electron: ' + relative)
            expected, actual = original.read(member), path.read_bytes()
            if member.filename == binary and platform == 'win32':
                expected, actual = pe_code(expected), pe_code(actual)
            if hashlib.sha256(actual).digest() != hashlib.sha256(expected).digest():
                raise ValueError('Componente Electron modificado sin fuentes correspondientes: ' + relative)


def copy_application(destination):
    # Lista explícita: nunca incluir node_modules, dist, perfiles ni datos del SO.
    source = ROOT / 'src/desktop'
    for pattern in ('*.cjs', '*.js', '*.json', '*.css', '*.html', '*.png', 'locales/*.json'):
        for path in source.glob(pattern):
            if path.is_symlink():
                raise ValueError('Enlace en las fuentes de la aplicación.')
            target = destination / 'src/desktop' / path.relative_to(source)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, target)
    for relative in ('LICENSE', 'NOTICE', 'VERSION', 'tools/desktop_compliance.py',
                     'packaging/desktop/README.md', 'packaging/desktop/electron-sources.lock.json'):
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / relative, target)


def prepare(runtime, platform, cache, lock=None):
    runtime, cache = Path(runtime).resolve(), Path(cache).resolve()
    lock = lock or json.loads(LOCK.read_text())
    record = lock['runtimes'][platform]
    archive = acquire(record, cache)
    if (runtime / 'version').read_text().strip() != lock['electron_version']:
        raise ValueError('La versión de Electron no coincide con las fuentes fijadas.')
    check_runtime(runtime, archive, platform)
    sources = [(record, acquire(record, cache)) for record in lock['sources']]
    # La carpeta legal anterior se conserva hasta completar todas las descargas.
    with tempfile.TemporaryDirectory(prefix='hgw-desktop-legal-', dir=runtime.parent) as temporary:
        stage = Path(temporary) / 'legal'
        (stage / 'sources').mkdir(parents=True)
        for record, source in sources:
            shutil.copy2(source, stage / 'sources' / record['name'])
        for name in ('LICENSE', 'LICENSES.chromium.html'):
            shutil.copy2(runtime / name, stage / name)
        shutil.copy2(ROOT / 'packaging/desktop/README.md', stage / 'README.md')
        copy_application(stage / 'application-source')
        # Vincula fuentes y runtime concreto: una versión igual no basta.
        shipped = {p.relative_to(runtime).as_posix(): digest(p) for p in files(runtime)
                   if 'legal' not in p.relative_to(runtime).parts}
        legal = {p.relative_to(stage).as_posix(): digest(p) for p in files(stage)}
        manifest = {'format': 1, 'platform': platform, 'lock': lock,
                    'runtime_files': shipped, 'legal_files': legal}
        (stage / 'manifest.json').write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
        target = runtime / 'legal'
        backup = Path(temporary) / 'previous'
        if target.is_symlink():
            raise ValueError('La carpeta legal no puede ser un enlace.')
        if target.exists():
            target.rename(backup)
        try:
            shutil.move(stage, target)
        except BaseException:
            if backup.exists():
                backup.rename(target)
            raise
    verify(runtime, platform, lock)


def verify(runtime, platform, lock=None):
    runtime = Path(runtime)
    lock = lock or json.loads(LOCK.read_text())
    legal = runtime / 'legal'
    manifest = json.loads((legal / 'manifest.json').read_text())
    if (manifest.get('format') != 1 or manifest.get('platform') != platform
            or manifest.get('lock') != lock):
        raise ValueError('El escritorio no corresponde a los fuentes fijados; vuelve a empaquetarlo.')
    expected = dict(manifest['runtime_files'])
    expected.update({'legal/' + name: value for name, value in manifest['legal_files'].items()})
    expected['legal/manifest.json'] = digest(legal / 'manifest.json')
    actual = {p.relative_to(runtime).as_posix(): digest(p) for p in files(runtime)}
    if not expected or actual != expected:
        raise ValueError('El runtime, los avisos o los fuentes han cambiado; no se puede distribuir.')
    for name in ('LICENSE', 'LICENSES.chromium.html', 'README.md',
                 'application-source/LICENSE', 'application-source/NOTICE',
                 'application-source/src/desktop/package.json',
                 'application-source/src/desktop/package-lock.json',
                 'application-source/src/desktop/pack.cjs'):
        if not (legal / name).is_file():
            raise ValueError('Falta un aviso o fuente de la aplicación: ' + name)
    if (legal / 'application-source/LICENSE').read_bytes() != (ROOT / 'LICENSE').read_bytes():
        raise ValueError('La licencia del cliente está desactualizada; vuelve a empaquetarlo.')
    for record in lock['sources']:
        if actual.get('legal/sources/' + record['name']) != record['sha256']:
            raise ValueError('Faltan los fuentes correspondientes: ' + record['name'])
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'verify'))
    parser.add_argument('runtime', type=Path)
    parser.add_argument('--platform', required=True, choices=('linux', 'win32'))
    parser.add_argument('--cache', type=Path, default=ROOT / 'builds/desktop-license-cache')
    args = parser.parse_args()
    try:
        if args.action == 'prepare':
            prepare(args.runtime, args.platform, args.cache)
        else:
            verify(args.runtime, args.platform)
    except (ValueError, OSError, KeyError, zipfile.BadZipFile) as error:
        parser.exit(1, str(error) + '\n')
    print('OK: fuentes y avisos de escritorio, ' + args.platform)


if __name__ == '__main__':
    main()
