#!/usr/bin/env python3
"""Genera paquetes completos de Home Assistant y YunoHost con binarios recién compilados."""
import argparse
import gzip
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
VERSION = (ROOT / 'VERSION').read_text().strip()


def copy_tree(source, destination):
    shutil.copytree(source, destination, ignore=shutil.ignore_patterns('__pycache__', '.pytest_cache', '*.pyc'))


def add_notices(destination):
    destination.mkdir(parents=True, exist_ok=True)
    for name in ('LICENSE', 'NOTICE'):
        shutil.copy2(ROOT / name, destination / name)
    # Conserva literalmente el inventario y los avisos existentes, sin el entorno
    # Electron de escritorio. El empaquetado no cambia ninguna licencia.
    for name in ('go', 'go-toolchain'):
        if (ROOT / 'src/third_party' / name).exists():
            copy_tree(ROOT / 'src/third_party' / name, destination / 'third_party' / name)
    inventory = (ROOT / 'src/third_party/inventory.json').read_text().replace('src/third_party/', 'third_party/')
    (destination / 'third_party/inventory.json').write_text(inventory)


def source_files():
    for path in sorted((ROOT / 'src').glob('*.go')):
        if not path.name.endswith('_test.go'):
            yield path
    for path in sorted((ROOT / 'src/internal').rglob('*')):
        if (path.is_file() and not path.is_symlink() and
            not any(part == '__pycache__' for part in path.parts) and
            not path.name.endswith(('_test.go', '.test.cjs', '.pyc'))):
            yield path
    for name in ('src/go.mod', 'src/go.sum', 'LICENSE', 'NOTICE', 'VERSION'):
        yield ROOT / name
    yield ROOT / 'packaging/common/SOURCE.md'


def export(output, archives=True):
    if output.exists() and any(output.iterdir()):
        raise SystemExit('La carpeta de salida debe estar vacía; los paquetes existentes no se sobrescriben')
    output.mkdir(parents=True, exist_ok=True)
    # Separa los repositorios y avisos exportados del recorrido ./... del módulo
    # Go padre cuando la salida está dentro del directorio de fuentes.
    (output / 'go.mod').write_text('module headscale-gateway-platform-artifacts\n\ngo 1.27.4\n')
    with tempfile.TemporaryDirectory(prefix='hgw-platforms-') as temporary:
        stage = Path(temporary)
        ha = stage / 'homeassistant'
        addon = ha / 'headscale_gateway'
        copy_tree(ROOT / 'packaging/homeassistant', addon)
        (addon / 'config.yaml.in').rename(addon / 'config.yaml')
        shutil.copy2(ROOT / 'packaging/common/hgw_bootstrap.py', addon / 'hgw_bootstrap.py')
        (ha / 'repository.yaml').write_text('name: Headscale Gateway\nmaintainer: gentedelcondado\n')
        (ha / 'README.md').write_text(
            '# Complementos de Headscale Gateway\n\n'
            'Para una instalación local, copia headscale_gateway/ a la carpeta de aplicaciones '
            'locales de Home Assistant. Consulta headscale_gateway/DOCS.md. '
            'Este paquete incluye las fuentes de compilación de su versión.\n')
        source_hashes = {}
        # Incluye dependencias con su código y avisos, no solo referencias remotas.
        # El vendor temporal no se escribe en las fuentes del proyecto.
        vendor = stage / 'vendor'
        subprocess.run(['go', 'mod', 'vendor', '-o', str(vendor)], cwd=ROOT / 'src', check=True)
        sources = [(path, str(path.relative_to(ROOT / 'src')) if path.is_relative_to(ROOT / 'src')
                    else path.name) for path in source_files()]
        sources.extend((path, 'vendor/' + path.relative_to(vendor).as_posix())
                       for path in sorted(vendor.rglob('*')) if path.is_file())
        # Produce los mismos bytes entre clones y compilaciones de las mismas fuentes.
        with (addon / 'source.tar.gz').open('wb') as stream:
            with gzip.GzipFile(fileobj=stream, mode='wb', filename='', mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode='w') as archive:
                    for path, relative in sources:
                        info = archive.gettarinfo(str(path), arcname=relative)
                        info.uid = info.gid = info.mtime = 0
                        info.uname = info.gname = ''
                        info.mode = 0o644
                        with path.open('rb') as source:
                            archive.addfile(info, source)
                        source_hashes[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
        add_notices(addon / 'notices')
        ynh = stage / 'yunohost'
        copy_tree(ROOT / 'packaging/yunohost', ynh)
        # El icono cuadrado existente sirve también para las fichas de YunoHost.
        shutil.copy2(ROOT / 'packaging/homeassistant/icon.png', ynh / 'doc/logo.png')
        shutil.copy2(ROOT / 'packaging/common/hgw_bootstrap.py', ynh / 'conf/hgw_bootstrap.py')
        payload = ynh / 'payload'
        payload.mkdir()
        # Los binarios públicos siempre viajan con las mismas fuentes que usa HA.
        shutil.copy2(addon / 'source.tar.gz', payload / 'source.tar.gz')
        add_notices(payload / 'notices')
        shutil.copy2(ROOT / 'LICENSE', ynh / 'LICENSE')
        binaries = {}
        subprocess.run(['go', 'mod', 'verify'], cwd=ROOT / "src", check=True)
        for arch in ('amd64', 'arm64'):
            binary = payload / ('headscale-gateway-linux-' + arch)
            subprocess.run(['go', 'build', '-mod=readonly', '-buildvcs=false', '-trimpath',
                            '-o', str(binary), '.'], cwd=ROOT / "src",
                           env=dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH=arch), check=True)
            binary.chmod(0o755)
            binaries[binary.name] = hashlib.sha256(binary.read_bytes()).hexdigest()
        (payload / 'SHA256SUMS').write_text(''.join(f'{digest}  {name}\n' for name, digest in binaries.items()))
        for script in (ynh / 'scripts').iterdir():
            script.chmod(0o755)
        repositories = ['homeassistant', 'yunohost']
        if (ROOT / 'packaging/hacs').exists():
            copy_tree(ROOT / 'packaging/hacs', stage / 'hacs')
            copy_tree(ROOT / 'custom_components', stage / 'hacs/custom_components')
            shutil.copy2(ROOT / 'hacs.json', stage / 'hacs/hacs.json')
            shutil.copy2(ROOT / 'LICENSE', stage / 'hacs/LICENSE')
            repositories.append('hacs')
        manifest = {'version': VERSION, 'commercial': False, 'sources': source_hashes, 'binaries': binaries}
        for name in repositories:
            (stage / name / 'build-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
            if archives:
                destination = output / f'headscale-gateway-{name}-{VERSION}.zip'
                with zipfile.ZipFile(destination, 'w', zipfile.ZIP_DEFLATED) as archive:
                    for path in sorted((stage / name).rglob('*')):
                        if path.is_file():
                            archive.write(path, str(path.relative_to(stage)))
            shutil.copytree(stage / name, output / name)
        sums = ''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n'
                       for p in sorted(output.glob('*.zip')))
        (output / 'SHA256SUMS').write_text(sums)
        (output / 'README.txt').write_text(
            'Paquetes de plataforma generados desde el mismo código del repositorio.\n'
            'Los binarios originales de uso local no se han sustituido.\n'
            'La integración de entidades está en custom_components/headscale_gateway/. '
            'Desde Forgejo privado se instala mediante copia local.\n'
            'Home Assistant compila el complemento con las fuentes incluidas. '
            'YunoHost incluye binarios Linux verificados.\n'
            'Lee el README de cada paquete antes de instalar. '
            'Sigue pendiente la aceptación completa en las plataformas reales.\n')
    print(output)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=ROOT / 'releases' / ('platforms-' + VERSION))
    parser.add_argument('--no-archives', action='store_true', help='Prepara las carpetas sin crear archivos ZIP')
    args = parser.parse_args()
    export(args.output.resolve(), archives=not args.no_archives)
