#!/usr/bin/env bash
set -euo pipefail

source_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
store_root=$(cd -- "${1:?Indica la carpeta del repositorio Mis-app}" && pwd -P)
app=headscale_gateway
source_path=addons/$app
destination=$store_root/$app

test "$source_root" != "$store_root"
test -f "$store_root/repository.yaml"
if [ -L "$destination" ]; then
    echo 'La carpeta de destino no puede ser un enlace simbólico.' >&2
    exit 1
fi

# Copiar solo el paquete versionado en el commit, sin archivos locales.
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
git -C "$source_root" archive HEAD -- "$source_path" | tar -xf - -C "$stage"
for required in config.yaml Dockerfile source.tar.gz icon.png logo.png; do
    test -s "$stage/$source_path/$required"
done
if [ -n "$(find "$stage/$source_path" -type l -print -quit)" ]; then
    echo 'El complemento contiene enlaces simbólicos.' >&2
    exit 1
fi

# Las eliminaciones se limitan a la carpeta que pertenece al Gateway.
rsync -a --delete -- "$stage/$source_path/" "$destination/"
