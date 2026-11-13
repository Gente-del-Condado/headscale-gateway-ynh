#!/usr/bin/env python3
"""Compila el motor común para Android; no genera ni instala una APK."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ARCHITECTURES = {
    'arm64-v8a': ('arm64', 'aarch64-linux-android'),
    'x86_64': ('amd64', 'x86_64-linux-android'),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ndk', required=True, type=Path, help='Carpeta del Android NDK instalado')
    parser.add_argument('--arch', choices=ARCHITECTURES, default='arm64-v8a')
    parser.add_argument('--output', type=Path, help='Archivo de salida; por defecto builds/android/<arquitectura>/libheadscale_gateway.so')
    args = parser.parse_args()
    goarch, target = ARCHITECTURES[args.arch]
    compilers = list((args.ndk / 'toolchains/llvm/prebuilt').glob(f'*/bin/{target}24-clang'))
    if len(compilers) != 1:
        parser.error('No se encuentra un único compilador del NDK para Android API 24')
    # El nombre del enlace incluye la arquitectura y la API del compilador.
    compiler = compilers[0].absolute()
    readelf = compiler.parent / 'llvm-readelf'
    if not readelf.is_file():
        parser.error('Falta llvm-readelf en el NDK')
    destination = (args.output or ROOT / 'builds/android' / args.arch / 'libheadscale_gateway.so').resolve()
    destination.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run([
        'go', 'build', '-mod=readonly', '-buildvcs=false', '-trimpath', '-buildmode=pie',
        '-ldflags=-s -w -extldflags=-Wl,-z,max-page-size=16384',
        '-o', str(destination), '.',
    ], cwd=ROOT / "src", env=dict(os.environ, CGO_ENABLED='1', GOOS='android', GOARCH=goarch,
                         CC=str(compiler)), check=True)
    headers = subprocess.check_output([str(readelf), '-lW', str(destination)], text=True)
    loads = [line.split() for line in headers.splitlines() if line.strip().startswith('LOAD ')]
    if not loads or any(int(line[-1], 0) < 16384 for line in loads):
        raise SystemExit('El ejecutable no cumple la alineación ELF de 16 KiB')
    digest = hashlib.sha256(destination.read_bytes()).hexdigest()
    destination.with_suffix('.so.sha256').write_text(f'{digest}  {destination.name}\n')
    print(f'Motor Android compilado: {destination}')
    print('Alineación de segmentos ELF: 16 KiB o superior')
    print('Este script solo compila el motor; utiliza build_android_apk.py para generar la APK.')


if __name__ == '__main__':
    main()
