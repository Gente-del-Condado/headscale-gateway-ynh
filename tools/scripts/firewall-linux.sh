#!/usr/bin/env bash
# Asistente de firewall opcional para Fedora/Debian. No se ejecuta al arrancar.
set -euo pipefail
PORT="${1:-8443}"
case "$PORT" in ''|*[!0-9]*) echo 'Puerto inválido' >&2; exit 2;; esac
if (( PORT<1 || PORT>65535 )); then echo 'Puerto fuera de rango' >&2; exit 2; fi
DISCOVERY="${2:-no}"
echo "Este asistente abrirá TCP $PORT en el firewall local y no afectará a 9091."
if [[ "$DISCOVERY" == 'discovery' ]]; then echo 'También UDP 47991 para descubrimiento LAN.'; fi
read -r -p 'Escribe ACEPTAR para continuar: ' answer
[[ "$answer" == 'ACEPTAR' ]] || { echo 'Cancelado'; exit 1; }
if command -v firewall-cmd >/dev/null 2>&1; then
  sudo firewall-cmd --permanent --add-port="$PORT/tcp"
  if [[ "$DISCOVERY" == 'discovery' ]]; then sudo firewall-cmd --permanent --add-port=47991/udp; fi
  sudo firewall-cmd --reload
elif command -v ufw >/dev/null 2>&1; then
  sudo ufw allow "$PORT/tcp"
  if [[ "$DISCOVERY" == 'discovery' ]]; then sudo ufw allow 47991/udp; fi
else
  echo 'No se detectó firewalld ni UFW. Revisa nftables manualmente.' >&2
  exit 2
fi
