# Configurar almacenamiento privado para una instalación de usuario de Headscale Gateway.
# No instala servicios ni solicita permisos elevados. Ejecutar con PowerShell normal.
$ErrorActionPreference = 'Stop'
$DataDir = Join-Path $env:LOCALAPPDATA 'HeadscaleGateway'
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$Sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls.exe $DataDir /inheritance:r /grant:r ("*{0}:(OI)(CI)F" -f $Sid) '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'No se pudo restringir el acceso a data_dir mediante NTFS ACL.' }
Write-Host "Datos protegidos para el usuario actual: $DataDir"
Write-Host 'Ejecuta headscale-gateway.exe normalmente; no requiere privilegios de administrador.'
Write-Host 'Si ya utilizas C:\ProgramData\HeadscaleGateway, no migra ni modifica esa instalación.'

# La instalación no abre entradas de firewall sin consentimiento. Si se
# necesitan servicios HTTPS desde la LAN, ofrecer el asistente con UAC.
if ($Host.UI -and $env:SESSIONNAME -ne 'Services') {
  $answer = Read-Host '¿Quieres autorizar el HTTPS LAN en el firewall privado (TCP 8443)? [s/N]'
  if ($answer -match '^[sSyY]$') {
    & (Join-Path $PSScriptRoot 'firewall-windows.ps1') -Port 8443
  }
}
