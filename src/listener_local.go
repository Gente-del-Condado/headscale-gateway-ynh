package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"

	"net"
)

// listenLocal abre el listener TCP utilizado para las conexiones HTTPS
// procedentes de la red LAN.
//
// El listener se crea sobre la dirección indicada mediante addr.
//
// Ejemplos:
//
//	0.0.0.0:443
//	192.168.1.18:443
//
// La función solamente crea el listener TCP. No configura TLS ni inicia
// el servidor HTTP; esas responsabilidades corresponden al http.Server
// que utiliza posteriormente este listener.
//
// Este listener es independiente de Tailscale y Headscale, por lo que
// puede utilizarse aunque la conexión VPN no esté disponible.
func listenLocal(addr string) (net.Listener, error) {
	diagnostics.

		// Registra en el log que se está intentando abrir el listener
		// en la dirección configurada.
		Emit("gateway", diagnostics.InfoLevel, "HGW-LISTENER-LOCAL-001", "Abriendo listener HTTPS LAN en %s...", addr)

	// Abre un listener TCP en la dirección indicada.
	//
	// Si addr corresponde, por ejemplo, a 0.0.0.0:443, el proceso
	// aceptará conexiones TCP en todas las interfaces disponibles
	// que permitan escuchar en ese puerto.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf(
			"escuchando HTTPS LAN en %s: %w",
			addr,
			err,
		)
	}
	diagnostics.

		// El listener se ha creado correctamente y queda preparado
		// para ser utilizado por el servidor HTTPS.
		Emit("gateway", diagnostics.InfoLevel, "HGW-LISTENER-LOCAL-002", "Listener HTTPS LAN activo en %s", addr)

	return ln, nil
}
