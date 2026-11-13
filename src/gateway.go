package main

import (
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"github.com/gentedelcondado/headscale-gateway/internal/portal"

	"net/http"
	"net/http/httputil"
	"sort"
	"strings"
)

// newGatewayHandler crea el handler HTTP principal del gateway.
//
// El handler recibe:
//   - nodeDomain: dominio base asociado al nodo Tailscale/Headscale.
//   - proxyMap: mapa que relaciona cada hostname de servicio con su
//     ReverseProxy correspondiente.
//
// El mismo handler se utiliza independientemente de si la petición
// llega por el listener LAN o por el listener Tailscale.
//
// La decisión de qué hacer con una petición se basa exclusivamente
// en el hostname solicitado:
//   - Si coincide con un servicio configurado, se envía al proxy.
//   - Si coincide con el dominio del nodo, se muestra el índice.
//   - Si no coincide con ninguno, se devuelve HTTP 404.
func newGatewayHandler(
	nodeDomain string,
	proxyMap map[string]*httputil.ReverseProxy,
	adminHost ...string,
) http.Handler {

	// http.HandlerFunc permite utilizar una función como implementación
	// del método ServeHTTP requerido por la interfaz http.Handler.
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			// Normaliza el Host recibido en la petición.
			//
			// Esto permite comparar el hostname de forma consistente
			// independientemente de diferencias como mayúsculas/minúsculas
			// o la presencia del puerto en r.Host.
			host := normalizeHost(r.Host)

			// Comprueba si el hostname corresponde a uno de los servicios
			// configurados en el gateway.
			//
			// Si existe un proxy asociado, la petición se entrega
			// directamente al ReverseProxy.
			if proxy, ok := proxyMap[host]; ok {
				diagnostics.Emit("proxy", diagnostics.Debug, "HGW-PROXY-REQ", "method=%s host=%s", r.Method, host)
				proxy.ServeHTTP(w, r)
				return
			}

			// Si la petición utiliza exactamente el dominio del nodo,
			// se muestra la página principal del gateway con el listado
			// de servicios disponibles.
			if host == nodeDomain {
				diagnostics.Emit("proxy", diagnostics.Trace, "HGW-PROXY-PORTAL", "portal request method=%s", r.Method)
				serveNodeIndex(
					w, r,
					nodeDomain,
					proxyMap, adminHost...,
				)
				return
			}
			diagnostics.

				// Si el hostname no corresponde ni al nodo ni a ningún
				// servicio configurado, se registra la petición para facilitar
				// el diagnóstico y se devuelve HTTP 404.
				Emit("proxy", diagnostics.Warn, "HGW-GATEWAY-001", "Host no encontrado: %s %s", r.Method,
					host,
				)

			http.Error(
				w,
				"Host no configurado",
				http.StatusNotFound,
			)
		},
	)
}

// El manejador independiente usa el mismo renderizador que el entorno configurable.
func serveNodeIndex(w http.ResponseWriter, r *http.Request, nodeDomain string, proxies map[string]*httputil.ReverseProxy, adminHost ...string) {
	entries := []portal.Entry{}
	for host := range proxies {
		id := strings.TrimSuffix(host, "."+nodeDomain)
		entries = append(entries, portal.Entry{ID: id, Name: strings.ReplaceAll(id, "-", " "), URL: "https://" + host, Gateway: nodeDomain})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	admin := ""
	if len(adminHost) > 0 && adminHost[0] != "" {
		admin = "https://" + adminHost[0] + "/admin/"
	}
	if err := portal.Render(w, r, portal.Default(), entries, admin); err != nil {
		diagnostics.Emit("gateway", diagnostics.Error, "HGW-GATEWAY-002", "portal de servicios: %v", err)
	}
}
