#!/usr/bin/env python3
"""Copia el manual generado únicamente a un checkout de la rama gh-pages."""
import argparse
from pathlib import Path
import shutil
import subprocess


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()


def publish(source, destination):
    source, destination = source.resolve(), destination.resolve()
    if source == destination or source in destination.parents or destination in source.parents:
        raise ValueError('El manual y su destino deben estar separados.')
    for relative in ('index.html', 'YUNOHOST.html', 'HOME_ASSISTANT.html', 'INTEGRACION_HA.html'):
        if not (source / relative).is_file():
            raise ValueError('Falta una página del manual: ' + relative)
    for path in source.rglob('*'):
        if path.is_symlink() or '.git' in path.relative_to(source).parts:
            raise ValueError('El manual no puede contener enlaces ni metadatos Git.')
    if (Path(git(destination, 'rev-parse', '--show-toplevel')).resolve() != destination
            or git(destination, 'branch', '--show-current') != 'gh-pages'):
        raise ValueError('El destino debe ser la raíz de otro checkout en gh-pages; nunca main.')
    # Solo se sustituye este checkout desechable; .git conserva el historial del manual.
    for path in destination.iterdir():
        if path.name == '.git':
            continue
        if path.is_dir() and not path.is_symlink():
            shutil.rmtree(path)
        else:
            path.unlink()
    shutil.copytree(source, destination, dirs_exist_ok=True)
    (destination / '.nojekyll').write_text('')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('destination', type=Path)
    args = parser.parse_args()
    try:
        publish(args.source, args.destination)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__':
    main()
