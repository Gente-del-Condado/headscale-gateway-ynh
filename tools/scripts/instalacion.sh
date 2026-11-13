#!/usr/bin/env bash
# Install a built Linux gateway and its private SQLite data directory.
set -euo pipefail
cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")/../.."
SERVICE_USER="${SERVICE_USER:-gateway}"
DATA_DIR=/var/lib/headscale-gateway
UNIT=/etc/systemd/system/headscale-gateway.service
case "$(uname -m)" in
  x86_64) BINARY=./bin/linux/headscale-gateway ;;
  aarch64|arm64) BINARY=./bin/linux-arm64/headscale-gateway ;;
  *) echo 'Arquitectura no admitida por este instalador' >&2; exit 1 ;;
esac
[[ -x "$BINARY" ]] || { echo 'Compila primero: ./build.sh' >&2;exit 1; }
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  sudo useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin "$SERVICE_USER"
fi
SERVICE_GROUP="$(id -gn "$SERVICE_USER")"
sudo install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 "$DATA_DIR"
sudo install -m 0755 "$BINARY" /usr/local/bin/headscale-gateway
TEMP_UNIT="$(mktemp)";trap 'rm -f "$TEMP_UNIT"' EXIT
sed -e "s/^User=.*/User=$SERVICE_USER/" -e "s/^Group=.*/Group=$SERVICE_GROUP/" \
  packaging/linux/headscale-gateway.service > "$TEMP_UNIT"
sudo install -m 0644 "$TEMP_UNIT" "$UNIT"
sudo systemctl daemon-reload
sudo systemctl enable --now headscale-gateway
printf '\nPanel local: http://127.0.0.1:9091/admin/\n'
printf 'Para el primer acceso o recuperación, detén el servicio e inicia en primer plano:\n'
printf '  sudo systemctl stop headscale-gateway\n'
printf '  sudo -u %s /usr/local/bin/headscale-gateway --db /var/lib/headscale-gateway/gateway.sqlite --reset-recovery\n' "$SERVICE_USER"
printf '  sudo -u %s /usr/local/bin/headscale-gateway --db /var/lib/headscale-gateway/gateway.sqlite\n' "$SERVICE_USER"
printf 'Completa el acceso local; luego Ctrl+C y sudo systemctl start headscale-gateway.\n'
printf 'El código temporal NO aparece en journalctl.\n' 
