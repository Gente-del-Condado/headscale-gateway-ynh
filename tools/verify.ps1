# Verificación desde PowerShell, compatible con Windows.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

python tools/reference_catalog.py --check
if ($LASTEXITCODE -ne 0) { throw 'Referencia desactualizada' }
python tools/check_i18n.py
if ($LASTEXITCODE -ne 0) { throw 'Traducciones incompletas' }
python tools/check_layout.py
if ($LASTEXITCODE -ne 0) { throw 'Interfaz inválida' }

Set-Location src
$goFiles = Get-ChildItem -Recurse -Filter '*.go' -File |
  Where-Object { $_.FullName -notmatch '[\\/](third_party|node_modules)[\\/]' }
$unformatted = @($goFiles | ForEach-Object { gofmt -l $_.FullName } | Where-Object { $_ })
if ($unformatted.Count -gt 0) { throw "Ejecutar gofmt: $($unformatted -join ', ')" }

Get-ChildItem internal/admin/*.js | ForEach-Object {
  node --check $_.FullName
  if ($LASTEXITCODE -ne 0) { throw "Error JS: $($_.Name)" }
}
Get-ChildItem internal/admin/*.test.cjs | ForEach-Object {
  node --test $_.FullName
  if ($LASTEXITCODE -ne 0) { throw "Prueba JS fallida: $($_.Name)" }
}
node --test desktop/client.test.cjs desktop/window-layout.test.cjs
if ($LASTEXITCODE -ne 0) { throw 'Tests mini GUI fallidos' }
$env:CGO_ENABLED = '0'
go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests Go fallidos' }
go test -tags commercial ./internal/licensing
if ($LASTEXITCODE -ne 0) { throw 'Tests de suscripciones fallidos' }
go test -tags commercial . -run '^TestCommercial'
if ($LASTEXITCODE -ne 0) { throw 'Protección de administración fallida' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'go vet falló' }
go build -buildvcs=false -o (Join-Path $env:TEMP 'headscale-gateway-verify.exe') .
if ($LASTEXITCODE -ne 0) { throw 'Compilación fallida' }
Write-Host 'OK: verificación completa'
