package main

import (
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
)

func printHelp() {
	fmt.Print(i18n.T(`Headscale Gateway — configuración centralizada en SQLite

USO:
  headscale-gateway [-db /ruta/gateway.sqlite]
  headscale-gateway [-db /ruta/gateway.sqlite] -reload
  headscale-gateway -backup /ruta/nueva/copia  (DESHABILITADO: utiliza copias .hgw cifradas)
  headscale-gateway -db /ruta/gateway.sqlite -restore-backup /ruta/copia (Gateway detenido)
  headscale-gateway -reset-recovery (rehabilita bootstrap local)
  headscale-gateway [-log-file /ruta/gateway.log] [-log-max-mb 10] [-log-backups 5]
  headscale-gateway [--log-level error|warn|info|debug|trace] [--debug|--trace]
  headscale-gateway logs status [--db ruta/gateway.sqlite]
  headscale-gateway logs tail [--channel gateway|proxy|admin] [--lines 50] [--dir /ruta/logs]
  headscale-gateway logs follow [--channel proxy] [--dir /ruta/logs]   (Ctrl+C)
  headscale-gateway doctor [--db ruta/gateway.sqlite]
  headscale-gateway version
  headscale-gateway -help

Linux: ~/.local/share/headscale-gateway/gateway.sqlite para usuario normal;
como root usa /var/lib/headscale-gateway/gateway.sqlite.
Windows: %LOCALAPPDATA%/HeadscaleGateway/gateway.sqlite, respetando una
instalación previa en %ProgramData%/HeadscaleGateway.
El primer inicio crea SQLite y sirve el panel local en:
  http://127.0.0.1:9091/admin/

AJUSTES Y SERVICIOS:
  Se administran desde el panel y la API. No existe config.yaml.
  La gestión de identidad, credenciales y roles utiliza la misma SQLite.
  -reload almacena una solicitud en SQLite, procesada por el servidor.

LOGS E IDIOMAS:
  Canales independientes: gateway.log, proxy.log, admin.log.
  Guarda 5 copias rotadas de 10 MiB por canal (configurable).
  INFO predeterminado. DEBUG/TRACE detallan solicitudes sin cuerpos ni cabeceras.
  Prioridad nivel: flags > GATEWAY_LOG_LEVEL > SQLite > INFO.
  En el panel Registros se puede activar DEBUG temporal hasta 60 minutos.
  CLI doctor/logs solo lee archivos y no inicia la VPN.
  Los eventos de API y plantillas tienen identificadores HGW-*.
  GATEWAY_LANG=es (predeterminado) o GATEWAY_LANG=en para los nuevos
  mensajes de eventos traducidos. El idioma del panel es independiente.
  Traducciones Poedit: locale/*.po y locale/headscale-gateway.pot.

DATOS EXTERNOS A SQLITE:
  Tailscale tsnet y CertMagic gestionan su propio estado y certificados.
  No modifiques manualmente esos archivos. Usa copias consistentes de SQLite.
  La restauración incluye gateway.sqlite y master.key; no restaura
  automáticamente las ACL ni la configuración YAML del VPS Headscale.
  Las copias .hgw actuales están cifradas con contraseña.

RED Y SEGURIDAD:
  El listener HTTP de administración utiliza loopback por defecto.
  Para acceso remoto usa HTTPS del Gateway o un proxy HTTPS explícitamente autorizado.
GATEWAY_BOOTSTRAP_FILE proporciona el mismo contrato de arranque en todas las instalaciones.
`))
}
