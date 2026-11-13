#!/usr/bin/env python3
"""Publica fuentes completos saneados en una rama sin historial privado."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

from repository_archive import ROOT, source_files

EXCLUDED = {'addons', 'yunohost', '.github', '.forgejo'}


def snapshot(destination):
    destination = Path(destination)
    if destination.exists() and any(destination.iterdir()):
        raise ValueError('La carpeta de fuentes debe estar vacía')
    destination.mkdir(parents=True, exist_ok=True)
    checks = []
    for source in source_files():
        relative = source.relative_to(ROOT)
        if relative.parts[0] in EXCLUDED or relative.name == 'git-install-manifest.json':
            continue
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
        checks.append(hashlib.sha256(target.read_bytes()).hexdigest() + '  ' + relative.as_posix())
    (destination / 'CONTENIDO.sha256').write_text('\n'.join(checks) + '\n')
    (destination / 'SOURCE_SNAPSHOT.md').write_text(
        '# Fuentes completos de Headscale Gateway\n\n'
        'Exportación sin historial privado, claves, datos ni resultados de compilación.\n'
        'El motor común está en src/ y Android en android/. Conserva LICENSE, NOTICE\n'
        'y los avisos de terceros. Sigue android/README.md para construir Android.\n'
        'Los instaladores generados se reconstruyen con tools/prepare_git_install.py.\n'
        'No se incluyen los flujos privados de publicación.\n')
    return len(checks)


def publish(repository, tag=False):
    # El destino es el clon del paquete ya autenticado por la clave limitada.
    repository = Path(repository).resolve()
    def source_git(*args):
        return subprocess.check_output(['git', '-C', str(repository), *args], text=True).strip()
    remote = source_git('remote', 'get-url', 'origin')
    ssh_command = source_git('config', '--get', 'core.sshCommand')
    with tempfile.TemporaryDirectory(prefix='hgw-public-sources-') as temporary:
        stage = Path(temporary)
        snapshot(stage)
        def git(*args):
            return subprocess.run(['git', '-C', str(stage), *args], check=True)
        git('init', '-b', 'upstream-source')
        git('config', 'core.sshCommand', ssh_command)
        git('config', 'user.name', 'github-actions[bot]')
        git('config', 'user.email', '41898282+github-actions[bot]@users.noreply.github.com')
        git('remote', 'add', 'origin', remote)
        previous = source_git('ls-remote', '--heads', 'origin', 'refs/heads/upstream-source')
        if previous:
            git('fetch', '--no-tags', 'origin', 'refs/heads/upstream-source')
            git('update-ref', 'refs/heads/upstream-source', 'FETCH_HEAD')
            git('reset', '--mixed', 'HEAD')
        git('add', '--all')
        changed = subprocess.run(['git', '-C', str(stage), 'diff', '--cached', '--quiet']).returncode
        if changed:
            version = (ROOT / 'VERSION').read_text().strip()
            git('commit', '-m', 'Publicar fuentes completos ' + version)
            git('push', 'origin', 'HEAD:refs/heads/upstream-source')
        if tag:
            name = 'source-v' + (ROOT / 'VERSION').read_text().strip()
            # Las etiquetas publicadas nunca se mueven al cambiar documentación.
            if not source_git('ls-remote', '--tags', 'origin', 'refs/tags/' + name):
                git('tag', name)
                git('push', 'origin', 'refs/tags/' + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument('--snapshot', type=Path)
    selection.add_argument('--publish', type=Path, help='Clon del paquete con clave SSH limitada')
    parser.add_argument('--tag', action='store_true')
    args = parser.parse_args()
    if args.snapshot:
        print(json.dumps({'files': snapshot(args.snapshot), 'path': str(args.snapshot)}))
    else:
        publish(args.publish, args.tag)


if __name__ == '__main__':
    main()
