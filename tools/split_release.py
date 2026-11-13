#!/usr/bin/env python3
"""Divide una entrega íntegra, sin retirar fuentes, para límites de adjuntos."""
import argparse
import hashlib
import json
from pathlib import Path


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def split(source, output, name, chunk_bytes):
    if source.is_symlink() or not source.is_file() or chunk_bytes < 1:
        raise ValueError('Archivo o tamaño de fragmento no válido')
    if output.exists() and any(output.iterdir()):
        raise ValueError('La carpeta de fragmentos debe estar vacía')
    output.mkdir(parents=True, exist_ok=True)
    checks = []
    combined = hashlib.sha256()
    with source.open('rb') as incoming:
        while True:
            first = incoming.read(min(1024 * 1024, chunk_bytes))
            if not first:
                break
            path = output / (name + '.part-' + str(len(checks) + 1).zfill(2))
            current = hashlib.sha256()
            remaining = chunk_bytes
            with path.open('wb') as fragment:
                block = first
                while block:
                    fragment.write(block)
                    current.update(block)
                    combined.update(block)
                    remaining -= len(block)
                    if not remaining:
                        break
                    block = incoming.read(min(1024 * 1024, remaining))
            checks.append({'name': path.name, 'bytes': path.stat().st_size, 'sha256': current.hexdigest()})
    # Leer de nuevo los fragmentos detecta también una escritura truncada.
    verified = hashlib.sha256()
    for item in checks:
        path = output / item['name']
        if digest(path) != item['sha256']:
            raise ValueError('Fragmento dañado: ' + path.name)
        with path.open('rb') as fragment:
            while block := fragment.read(1024 * 1024):
                verified.update(block)
    if not checks or verified.hexdigest() != combined.hexdigest():
        raise ValueError('La reconstrucción no coincide con la entrega original')
    report = {'archive': name, 'sha256': combined.hexdigest(), 'parts': checks}
    (output / 'parts.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--chunk-bytes', type=int, default=2100000000)
    args = parser.parse_args()
    print(json.dumps(split(args.source, args.output, args.source.name, args.chunk_bytes), indent=2))


if __name__ == '__main__':
    main()
