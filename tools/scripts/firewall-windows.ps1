# Asistente OPCIONAL de reglas LAN. Uso (PowerShell):
#   .\scrip\firewall-windows.ps1 -Port 8443 -Discovery
# Solo funciona con UAC aprobado; jamás lo ejecuta silenciosamente el Gateway.
param(
  [ValidateRange(1,65535)][int]$Port = 8443,
  [switch]$Discovery
)
$ErrorActionPreference = 'Stop'
$elevated = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $elevated) {
  Write-Host 'Se solicitará permiso UAC para modificar exclusivamente el firewall privado.'
  $args = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $PSCommandPath + '"'), '-Port', [string]$Port)
  if ($Discovery) { $args += '-Discovery' }
  Start-Process -FilePath 'powershell.exe' -ArgumentList $args -Verb RunAs -Wait
  exit
}
Write-Host "Regla TCP LAN para puerto $Port (solo perfil Private)."
if ($Discovery) { Write-Host 'Además, UDP 47991 para descubrimiento local.' }
$accept = Read-Host 'Escribe ACEPTAR para instalar las reglas'
if ($accept -cne 'ACEPTAR') { Write-Host 'Cancelado. No se han añadido reglas.'; exit 1 }
$ruleName = "HeadscaleGateway-LAN-$Port-TCP"
if (-not (Get-NetFirewallRule -Name $ruleName -ErrorAction SilentlyContinue)) {
  New-NetFirewallRule -Name $ruleName -DisplayName $ruleName -Enabled True -Profile Private -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port | Out-Null
}
if ($Discovery) {
  $name = 'HeadscaleGateway-Discovery-47991-UDP'
  if (-not (Get-NetFirewallRule -Name $name -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name $name -DisplayName $name -Enabled True -Profile Private -Direction Inbound -Action Allow -Protocol UDP -LocalPort 47991 | Out-Null
  }
}
Write-Host 'Reglas instaladas. Puedes retirarlas desde Firewall de Windows o Remove-NetFirewallRule -Name ...'
