#!/usr/bin/env python3
"""Publica solo el paquete YunoHost versionado, sin copiar el repositorio privado."""
import argparse
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tarfile
import tempfile
import tomllib

ROOT = Path(__file__).resolve().parents[1]
MANAGED = ('LICENSE', 'README.md', 'build-manifest.json', 'conf', 'doc', 'go.mod',
           'manifest.toml', 'payload', 'scripts', 'tests.toml')
BINARIES = ('headscale-gateway-linux-amd64', 'headscale-gateway-linux-arm64')


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args])


def validate_archive(data, stage):
    # Revisar todos los miembros antes de extraer: no seguir enlaces del paquete.
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:') as archive:
        for member in archive.getmembers():
            name = PurePosixPath(member.name)
            if (name.is_absolute() or '..' in name.parts or not name.parts
                    or name.parts[0] not in MANAGED
                    or not (member.isdir() or member.isfile())):
                raise ValueError('El paquete contiene una ruta o un tipo de archivo no publicable: ' + member.name)
        for member in archive.getmembers():
            path = stage.joinpath(*PurePosixPath(member.name).parts)
            if member.isdir():
                path.mkdir(parents=True, exist_ok=True)
            else:
                path.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as incoming, path.open('wb') as output:
                    shutil.copyfileobj(incoming, output)
                path.chmod(0o755 if member.mode & 0o111 else 0o644)


def validate_package(stage, version):
    required = ('LICENSE', 'README.md', 'manifest.toml', 'tests.toml', 'build-manifest.json',
                'scripts/install', 'scripts/upgrade', 'scripts/backup', 'scripts/restore',
                'conf/hgw_bootstrap.py', 'doc/logo.png', 'payload/SHA256SUMS', 'payload/notices/LICENSE',
                'payload/notices/NOTICE', 'payload/notices/third_party/inventory.json',
                'payload/source.tar.gz')
    for relative in required:
        if not (stage / relative).is_file() or not (stage / relative).stat().st_size:
            raise ValueError('Falta un archivo necesario del paquete: ' + relative)
    manifest = tomllib.loads((stage / 'manifest.toml').read_text())
    if manifest.get('id') != 'headscale_gateway' or manifest.get('version', '').split('~')[0] != version:
        raise ValueError('El identificador o la versión YunoHost no coinciden con el motor.')
    build = json.loads((stage / 'build-manifest.json').read_text())
    if build.get('version') != version or build.get('commercial') is not False:
        raise ValueError('El manifiesto de compilación no corresponde a esta distribución.')
    # No publicar binarios sin las fuentes exactas declaradas por su generador.
    with tarfile.open(stage / 'payload/source.tar.gz', 'r:gz') as archive:
        sources = {}
        for member in archive.getmembers():
            name = PurePosixPath(member.name)
            if (not member.isfile() or name.is_absolute() or '..' in name.parts
                    or member.name in sources):
                raise ValueError('Archivo de fuentes no válido: ' + member.name)
            with archive.extractfile(member) as stream:
                sources[member.name] = hashlib.file_digest(stream, 'sha256').hexdigest()
    if not sources or sources != build.get('sources'):
        raise ValueError('Las fuentes no coinciden con el manifiesto de compilación.')
    expected = {}
    for line in (stage / 'payload/SHA256SUMS').read_text().splitlines():
        digest, name = line.split()
        if name not in BINARIES or name in expected or len(digest) != 64:
            raise ValueError('El listado de sumas de binarios no es válido.')
        expected[name] = digest
    if set(expected) != set(BINARIES) or expected != build.get('binaries'):
        raise ValueError('Los dos manifiestos no describen los mismos binarios.')
    for name, digest in expected.items():
        binary = stage / 'payload' / name
        if not binary.is_file() or hashlib.sha256(binary.read_bytes()).hexdigest() != digest:
            raise ValueError('El binario no coincide con su suma: ' + name)
        if not binary.stat().st_mode & 0o111:
            raise ValueError('El binario no tiene permiso de ejecución: ' + name)


def publish(source, destination):
    source, destination = source.resolve(), destination.resolve()
    if destination == source or source in destination.parents or destination in source.parents:
        raise ValueError('El destino debe ser un checkout separado del repositorio principal.')
    if Path(git(destination, 'rev-parse', '--show-toplevel').decode().strip()).resolve() != destination:
        raise ValueError('El destino debe ser la raíz de su propio repositorio Git.')
    version = git(source, 'show', 'HEAD:VERSION').decode().strip()
    data = git(source, 'archive', '--format=tar', 'HEAD:yunohost')
    with tempfile.TemporaryDirectory(prefix='hgw-ynh-publish-') as temporary:
        stage = Path(temporary)
        validate_archive(data, stage)
        validate_package(stage, version)
        # Todas las validaciones terminan antes de sustituir ningún archivo.
        for name in MANAGED:
            target = destination / name
            if target.is_symlink():
                raise ValueError('El destino administrado no puede ser un enlace: ' + name)
        for name in MANAGED:
            target = destination / name
            if target.is_dir():
                shutil.rmtree(target)
            elif target.exists():
                target.unlink()
            origin = stage / name
            if origin.is_dir():
                shutil.copytree(origin, target)
            elif origin.is_file():
                shutil.copy2(origin, target)
    # Archivos ajenos (por ejemplo .github/) y .git quedan fuera del ámbito gestionado.
    print('Paquete YunoHost ' + version + ' preparado desde ' + git(source, 'rev-parse', 'HEAD').decode().strip())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('destination', type=Path, help='Checkout de headscale-gateway_ynh')
    args = parser.parse_args()
    try:
        publish(ROOT, args.destination)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__':
    main()
