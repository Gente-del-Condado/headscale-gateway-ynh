#!/usr/bin/env bash
# Comprobación del repositorio sin modificar datos de una instalación real.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 tools/check_repository.py
python3 tools/check_layout.py
python3 tools/set_version.py --check
python3 tools/reference_catalog.py --check
python3 tools/check_i18n.py
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
python3 tools/prepare_git_install.py --check
cd src
unformatted="$(find . -name '*.go' -not -path './third_party/*' -not -path './desktop/node_modules/*' -print0 | xargs -0 gofmt -l)"
if [[ -n "$unformatted" ]]; then printf 'Archivos sin gofmt:\n%s\n' "$unformatted"; exit 1; fi
for script in internal/admin/*.js; do node --check "$script"; done
node --test internal/admin/*.test.cjs
node --test desktop/*.test.cjs
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test -tags commercial ./internal/licensing
CGO_ENABLED=0 go test -tags commercial . -run '^TestCommercial'
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go build -buildvcs=false -o "${TMPDIR:-/tmp}/headscale-gateway-verify" .
echo 'OK: verificación completa'
