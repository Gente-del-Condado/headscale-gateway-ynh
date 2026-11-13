# Go 1.27.4+, Python 3.11+. No dependency upgrades or source rewrites.
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    python tools/build_release.py @args
    if ($LASTEXITCODE -ne 0) { throw 'Compilación fallida' }
} finally {
    Pop-Location
}
