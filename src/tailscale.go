package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"fmt"

	"path/filepath"
	"strings"
	"time"

	"tailscale.com/tsnet"
)

// newTailscaleServer crea e inicializa una instancia de tsnet.Server.
//
// Esta instancia permite que el Gateway funcione como un nodo
// Tailscale independiente del cliente Tailscale del sistema.
//
// La configuración procede de Config (generada desde SQLite)
// y del directorio de estado indicado.
//
// La conexión con Headscale tiene un tiempo máximo de espera de
// 5 minutos. Si no se consigue establecer la conexión dentro
// de ese tiempo, la función devuelve un error y el llamador puede
// continuar funcionando en modo LAN.
func newTailscaleServer(
	cfg Config,
	hostname string,
	stateDir string,
	onCreated func(*tsnet.Server),
	onAuthURL func(string),
) (*tsnet.Server, error) {
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-001",

		// Crea la instancia de tsnet que representará al Gateway
		// dentro de la red Tailscale.
		"Iniciando tsnet...")

	tsServer := &tsnet.Server{
		// Nombre solicitado inicialmente para el nodo.
		//
		// Este valor no se considera necesariamente la identidad
		// final del nodo, ya que Headscale puede asignar otro nombre.
		Hostname: hostname,

		// URL del servidor de control Headscale (headscale.url).
		ControlURL: cfg.HeadscaleURL,

		// Directorio donde tsnet almacenará su estado persistente.
		Dir: stateDir,

		// Redirige los mensajes internos de tsnet al logger principal
		// de la aplicación, identificándolos con el prefijo [tsnet].
		UserLogf: func(format string, args ...any) {
			line := fmt.Sprintf(format, args...)
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TSNET-DETAIL", "Detalle de tsnet: %s", line)
			if onAuthURL != nil {
				if link := gatewayLoginURLFromLog(cfg.HeadscaleURL, line); link != "" {
					onAuthURL(link)
				}
			}
		},
	}

	if onCreated != nil {
		onCreated(tsServer)
	}
	// Crea un contexto con un tiempo máximo de espera de 5 minutos
	// para la inicialización de tsnet.
	//
	// Esto evita que una caída o inaccesibilidad de Headscale
	// bloquee indefinidamente el arranque del Gateway.
	ctx, cancel := context.WithTimeout(
		processContext,
		5*time.Minute,
	)
	defer cancel()

	// Inicializa tsnet y espera hasta que se complete la conexión
	// o hasta que expire el contexto.
	if _, err := tsServer.Up(ctx); err != nil {

		// Si la inicialización falla, cerramos la instancia para
		// liberar los recursos que pudiera haber creado tsnet.
		tsServer.Close()

		return nil, fmt.Errorf(
			"tsnet.Up: %w",
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-003", "tsnet iniciado correctamente")

	return tsServer, nil
}

// tailscaleDNSName obtiene el FQDN real asignado al nodo por
// Tailscale/Headscale.
//
// Importante:
//
// tsnet.Server.Hostname contiene el nombre solicitado al iniciar
// el nodo. Ese nombre puede no coincidir con el nombre finalmente
// asignado por Headscale.
//
// Por ejemplo:
//
//	solicitado: equipo
//	asignado:   equipo-1.intra.example.net.
//
// La fuente de verdad es Status().Self.DNSName.
func tailscaleDNSName(
	tsServer *tsnet.Server,
) (string, error) {

	// No es posible consultar la identidad si no existe una
	// instancia de tsnet.
	if tsServer == nil {
		return "", fmt.Errorf(
			"tsnet server es nil",
		)
	}

	// Limita la consulta para no bloquear indefinidamente el supervisor.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Obtiene el cliente local que permite consultar el estado
	// de la instancia de tsnet.
	localClient, err := tsServer.LocalClient()
	if err != nil {
		return "", fmt.Errorf(
			"obteniendo LocalClient de tsnet: %w",
			err,
		)
	}

	// Consulta el estado actual de tsnet.
	status, err := localClient.Status(ctx)
	if err != nil {
		return "", fmt.Errorf(
			"obteniendo estado de tsnet: %w",
			err,
		)
	}

	// Comprueba que tsnet haya devuelto realmente un estado.
	if status == nil {
		return "", fmt.Errorf(
			"tsnet devolvió un estado nil",
		)
	}

	// Self contiene la información correspondiente al propio
	// nodo Tailscale.
	if status.Self == nil {
		return "", fmt.Errorf(
			"tsnet no tiene información del nodo local",
		)
	}

	// Obtiene el DNSName real asignado al nodo.
	//
	// Tailscale puede devolver el FQDN terminado en un punto,
	// por lo que se elimina dicho punto final.
	dnsName := strings.TrimSuffix(
		strings.TrimSpace(status.Self.DNSName),
		".",
	)

	// Un DNSName vacío no proporciona una identidad válida.
	if dnsName == "" {
		return "", fmt.Errorf(
			"tsnet no devolvió DNSName para el nodo local",
		)
	}
	diagnostics.

		// Registra la identidad real descubierta para facilitar
		// el diagnóstico del arranque y de la asociación con Headscale.
		Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-004", "Identidad Tailscale real: %s", dnsName)

	return dnsName, nil
}

// prepareDataDirs prepara los directorios utilizados por Tailscale
// y CertMagic para una identidad determinada.
//
// Devuelve:
//
//	stateDir -> directorio de estado persistente de tsnet.
//	certDir  -> directorio de almacenamiento de CertMagic.
//
// El hostname se normaliza antes de utilizarlo como parte de los
// nombres de directorio.
//
// Esta función centraliza la preparación de ambos directorios,
// aunque actualmente el arranque principal puede realizar esta
// preparación directamente.
func prepareDataDirs(
	dataDir string,
	hostname string,
) (stateDir string, certDir string, err error) {

	// Normaliza el hostname antes de incorporarlo a los nombres
	// de los directorios.
	normalizedHostname := normalizeDNSName(hostname)

	// Directorio donde tsnet almacenará su estado persistente.
	stateDir = filepath.Join(
		dataDir,
		"tailscale-state-"+normalizedHostname,
	)

	// Directorio utilizado como almacenamiento persistente
	// para CertMagic.
	certDir = filepath.Join(
		dataDir,
		"certmagic-"+normalizedHostname,
	)

	// Crea el directorio de estado con los permisos privados
	// definidos por mkdirPrivate().
	if err := mkdirPrivate(stateDir); err != nil {
		return "", "", fmt.Errorf(
			"creando state directory %s: %w",
			stateDir,
			err,
		)
	}

	// Crea el directorio de almacenamiento de CertMagic.
	if err := mkdirPrivate(certDir); err != nil {
		return "", "", fmt.Errorf(
			"creando CertMagic directory %s: %w",
			certDir,
			err,
		)
	}
	diagnostics.

		// Registra las rutas finales utilizadas para facilitar
		// el diagnóstico de la configuración y del almacenamiento.
		Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-005", "Estado Tailscale: %s", stateDir)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-006", "Almacén CertMagic: %s", certDir)

	return stateDir, certDir, nil
}
