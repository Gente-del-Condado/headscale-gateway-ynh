#!/usr/bin/env python3
"""Genera entregas de fuentes y ejecutables comprobados, sin datos privados."""
import argparse
import hashlib
import json
from pathlib import Path
import tarfile
import tempfile
import zipfile

from build_release import TARGETS, source_fingerprint
from package_platforms import add_notices
from repository_archive import ROOT, export
from desktop_compliance import verify as verify_desktop


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def write_archive(destination, pairs, windows=False):
    if destination.exists():
        raise ValueError('La entrega ya existe: ' + str(destination))
    if windows:
        with zipfile.ZipFile(destination, 'w', zipfile.ZIP_DEFLATED) as archive:
            for source, name in pairs:
                # Los fuentes Chromium ya están comprimidos y requieren ZIP64.
                archive.write(source, name, compress_type=zipfile.ZIP_STORED
                              if source.name.endswith(('.tar.xz', '.tar.gz')) else zipfile.ZIP_DEFLATED)
    else:
        with tarfile.open(destination, 'w:gz', compresslevel=1) as archive:
            for source, name in pairs:
                archive.add(source, arcname=name, recursive=False)
    return destination


def runtime_archives(output, server_only):
    manifest = json.loads((ROOT / 'bin/build-manifest.json').read_text())
    version = (ROOT / 'VERSION').read_text().strip()
    fingerprint = source_fingerprint()
    if not manifest or any(entry.get('version') != version or entry.get('source_fingerprint') != fingerprint for entry in manifest):
        raise ValueError('Ejecutables de otra versión o fuentes: repite tools/build_release.py')
    verified = {entry['path']: entry['sha256'] for entry in manifest}
    artifacts = []
    with tempfile.TemporaryDirectory(prefix='hgw-release-notices-') as temporary:
        notices = Path(temporary)
        add_notices(notices)
        common = [(p, p.relative_to(notices).as_posix())
                  for p in sorted(notices.rglob('*')) if p.is_file()]
        common.extend((ROOT / name, name) for name in ('VERSION', 'tools/configure.py',
                      'packaging/common/hgw_bootstrap.py'))
        common.append((ROOT / 'docs/LEEME_BINARIOS.txt', 'LEEME_BINARIOS.txt'))
        # Conserva las capturas y demás recursos enlazados por los manuales.
        common.extend((p, p.relative_to(ROOT).as_posix())
                      for p in sorted((ROOT / 'docs').rglob('*')) if p.is_file())
        for target in sorted({entry['target'] for entry in manifest}):
            system, _, directory = TARGETS[target]
            suffix = '.exe' if system == 'windows' else ''
            pairs = []
            for program in ('headscale-gateway', 'hgw-ca-helper'):
                source = ROOT / directory / (program + suffix)
                relative = source.relative_to(ROOT).as_posix()
                if verified.get(relative) != digest(source):
                    raise ValueError('Binario no verificado: ' + relative)
                pairs.append((source, program + suffix))
            extension = '.zip' if system == 'windows' else '.tar.gz'
            artifacts.append(write_archive(output / ('headscale-gateway-' + target + extension),
                                           pairs + common, windows=system == 'windows'))
        if not server_only:
            artifacts.extend(desktop_archives(output))
    return artifacts


def desktop_archives(output):
    artifacts = []
    for folder, name, windows in (
            ('bin/Gateway-Tray', 'gateway-tray-windows-x64.zip', True),
            ('bin/linux/Gateway-Tray', 'gateway-tray-linux-x64.tar.gz', False)):
        base = ROOT / folder
        if base.is_dir():
            verify_desktop(base, 'win32' if windows else 'linux')
            pairs = [(p, 'Gateway-Tray/' + p.relative_to(base).as_posix())
                     for p in sorted(base.rglob('*')) if p.is_file()]
            artifacts.append(write_archive(output / name, pairs, windows))
    if not artifacts:
        raise ValueError('No hay escritorios preparados para distribuir.')
    return artifacts


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--source-only', action='store_true', help='Solo fuentes e instaladores privados')
    parser.add_argument('--server-only', action='store_true', help='Excluir el cliente de bandeja')
    parser.add_argument('--desktop-only', action='store_true', help='Solo escritorios con sus fuentes y avisos')
    parser.add_argument('--complete-project', action='store_true', help='Añadir fuentes con las entregas Android verificadas')
    args = parser.parse_args()
    output = args.output.resolve()
    if (output == ROOT or ROOT in output.parents) and not output.is_relative_to(ROOT / 'releases'):
        parser.error('Utiliza releases/ o una carpeta fuera del repositorio')
    if output.exists() and any(output.iterdir()):
        parser.error('La carpeta de salida debe estar vacía')
    output.mkdir(parents=True, exist_ok=True)
    if args.desktop_only and (args.source_only or args.server_only or args.complete_project):
        parser.error('--desktop-only no se combina con otras selecciones de entrega')
    artifacts = [] if args.desktop_only else [export(output / 'headscale-gateway-fuentes.zip')]
    if args.desktop_only:
        artifacts.extend(desktop_archives(output))
    elif not args.source_only:
        artifacts.extend(runtime_archives(output, args.server_only))
    if args.complete_project:
        artifacts.append(export(output / 'headscale-gateway-proyecto-completo.zip', include_releases=True))
    (output / 'SHA256SUMS').write_text(''.join(digest(p) + '  ' + p.name + '\n' for p in artifacts))
    (output / 'CONTENIDO.txt').write_text(
        ('Escritorios: fuentes, parches, avisos y sumas en legal/ dentro de cada paquete.\n'
         if args.desktop_only else 'Fuentes: consulta CONTENIDO.sha256 dentro del ZIP.\n') +
        'Sin datos privados, cachés ni historial Git.\n'
        + ('Los ejecutables del servidor y helper están en la raíz de cada paquete.\n'
           if not args.desktop_only else 'Conserva Gateway-Tray/legal/ al redistribuir el escritorio.\n') +
        'La firma estable Android y los datos de las instalaciones se conservan fuera de Git.\n'
        'Consulta README.md y docs/MIGRACION_REPOSITORIO.md.\n')
    print(json.dumps({'artifacts': [p.name for p in artifacts], 'output': str(output)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
