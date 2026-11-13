#!/usr/bin/env bash
# Go 1.27.4+, Python 3.11+. Builds all six server/helper targets without changing sources.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
exec python3 tools/build_release.py "$@"
