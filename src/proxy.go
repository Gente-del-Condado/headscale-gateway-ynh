package main

// Mapeo de servicios a proxies, con opciones por servicio.
import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// buildProxyMap construye el mapa de ReverseProxy que utilizará
// el Gateway para enrutar las peticiones.
//
// Cada entrada de servicios procede del catálogo SQLite:
//
// services:
//
//	home-assistant:
//	  backend: "http://192.168.1.8:80"
//
// config.go transforma esa estructura en un map[string]string:
//
//	home-assistant -> http://192.168.1.8:80
//
// El nombre del servicio se normaliza y se combina con nodeDomain
// para construir el hostname público utilizado por el Gateway:
//
//	home-assistant.equipo-1.intra.example.net
//
// El resultado es un mapa donde la clave es el hostname completo
// y el valor es el ReverseProxy correspondiente.
func buildProxyMap(
	services map[string]string,
	nodeDomain string,
	transport http.RoundTripper,
) map[string]*httputil.ReverseProxy {

	// Inicializa el mapa que contendrá los proxies.
	proxyMap := make(
		map[string]*httputil.ReverseProxy,
	)

	// Recorre todos los servicios configurados.
	for serviceName, backend := range services {

		// Normaliza el nombre del servicio para obtener una
		// etiqueta DNS consistente.
		serviceName = normalizeServiceName(serviceName)

		// Convierte la URL del backend en un objeto *url.URL.
		target, err := url.Parse(backend)
		if err != nil {
			diagnostics.
				// Un servicio con una URL inválida no impide crear
				// el resto de proxies.
				//
				// El servicio problemático simplemente se omite.
				Emit("proxy", diagnostics.Warn, "HGW-PROXY-001", "ignorando servicio %q: URL inválida %q: %v", serviceName,
					backend,
					err,
				)
			continue
		}

		// Construye el hostname completo que utilizará el cliente
		// para acceder al servicio.
		//
		// Ejemplo:
		//
		// home-assistant + "." +
		// equipo-1.intra.example.net
		//
		// = home-assistant.equipo-1.intra.example.net
		fullDomain := serviceName + "." + nodeDomain

		// Crea el ReverseProxy asociado al servicio y lo almacena
		// utilizando el hostname completo como clave.
		proxyMap[fullDomain] = newReverseProxy(
			serviceName,
			target,
			transport,
		)
		diagnostics.

			// Registra la ruta de proxy en el log para facilitar
			// la comprobación de la configuración durante el arranque.
			Emit("proxy", diagnostics.InfoLevel, "HGW-PROXY-002", "Proxy: https://%s -> %s", fullDomain,
				target.String(),
			)
	}

	return proxyMap
}

// newReverseProxy crea un ReverseProxy para un servicio concreto.
//
// serviceName identifica el servicio dentro del Gateway.
//
// target contiene la URL del backend al que se reenviarán
// las peticiones.
//
// transport es el RoundTripper compartido utilizado para
// realizar las conexiones hacia los backends.
//
// El proxy conserva el Host original solicitado por el cliente,
// añade las cabeceras X-Forwarded y añade X-Gateway-Service
// para identificar el servicio que ha procesado la petición.
func newReverseProxy(
	serviceName string,
	target *url.URL,
	transport http.RoundTripper,
) *httputil.ReverseProxy {

	return &httputil.ReverseProxy{

		// Utiliza el transporte HTTP compartido por todos los servicios.
		Transport: transport,

		// Rewrite modifica la petición antes de enviarla al backend.
		Rewrite: func(pr *httputil.ProxyRequest) {

			// Guarda el Host original solicitado por el cliente.
			//
			// Esto permite conservar el hostname del Gateway aunque
			// la URL de destino se cambie al backend.
			originalHost := pr.In.Host

			// Cambia la URL de salida para apuntar al backend configurado.
			pr.SetURL(target)

			// Mantiene como Host de la petición de salida el Host
			// original recibido por el Gateway.
			pr.Out.Host = originalHost

			// Añade las cabeceras X-Forwarded correspondientes,
			// permitiendo al backend conocer información sobre
			// la petición original.
			pr.SetXForwarded()

			// Identifica explícitamente el servicio que ha procesado
			// esta petición.
			pr.Out.Header.Set(
				"X-Gateway-Service",
				serviceName,
			)
		},

		// ErrorHandler se ejecuta cuando el proxy no puede completar
		// correctamente la comunicación con el backend.
		//
		// El error se registra en el log y el cliente recibe HTTP 502.
		ErrorHandler: func(
			w http.ResponseWriter,
			r *http.Request,
			err error,
		) {
			diagnostics.Emit("proxy", diagnostics.Error, "HGW-PROXY-003", "proxy [%s] %s %s: %v", serviceName,
				r.Method,
				r.URL.RequestURI(),
				err,
			)

			// HTTP 502 Bad Gateway indica que el Gateway no ha podido
			// obtener correctamente la respuesta del servicio backend.
			http.Error(
				w,
				"Servicio temporalmente no disponible",
				http.StatusBadGateway,
			)
		},

		// ModifyResponse se ejecuta cuando el backend ha respondido
		// correctamente y la respuesta está disponible para el proxy.
		ModifyResponse: func(
			response *http.Response,
		) error {
			diagnostics.

				// Registra el código HTTP devuelto por el backend.
				Emit("proxy", diagnostics.Debug, "HGW-PROXY-004", "Proxy [%s] -> HTTP %d", serviceName,
					response.StatusCode,
				)

			// nil indica que no se modifica ni se rechaza la respuesta.
			return nil
		},
	}
}

// buildProxyMapAdvanced mantiene el contrato de nombres del Gateway, pero
// asigna a cada servicio su propio transporte y políticas HTTP.
func buildProxyMapAdvanced(services map[string]string, options map[string]ProxyOptions, domain string, tailnetDial ...func(context.Context, string, string) (net.Conn, error)) map[string]*httputil.ReverseProxy {
	result := make(map[string]*httputil.ReverseProxy, len(services))
	var transports []*http.Transport
	for name, backend := range services {
		target, err := url.Parse(backend)
		if err != nil {
			diagnostics.Emit("proxy", diagnostics.Error, "HGW-PROXY-005", "proxy %s: %v", name, err)
			continue
		}
		o, ok := options[name]
		if !ok {
			o = ProxyOptions{PreserveHost: true, ResponseHeaderTimeout: 30 * time.Second, DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 20}
		}
		tr := newHTTPTransport()
		transports = append(transports, tr)
		if o.EgressMode == "tailnet" {
			// IMPORTANTE: no utilizar HTTP(S)_PROXY del entorno ni hacer
			// fallback al sistema, pues cambiaría la IP de origen.
			tr.Proxy = nil
			if len(tailnetDial) == 0 || tailnetDial[0] == nil {
				tr.DialContext = func(context.Context, string, string) (net.Conn, error) {
					return nil, fmt.Errorf("backend %s: salida tailnet sin servidor tsnet disponible", name)
				}
			} else {
				// También la red integrada respeta dial_timeout por servicio.
				dial := tailnetDial[0]
				tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if o.DialTimeout > 0 {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, o.DialTimeout)
						defer cancel()
					}
					return dial(ctx, network, address)
				}
			}
		} else {
			tr.DialContext = (&net.Dialer{Timeout: o.DialTimeout, KeepAlive: 30 * time.Second}).DialContext
		}
		// No sustituir tr.DialContext después de seleccionar el transporte.
		tr.ResponseHeaderTimeout = o.ResponseHeaderTimeout
		tr.TLSHandshakeTimeout = o.TLSHandshakeTimeout
		tr.IdleConnTimeout = o.IdleConnTimeout
		tr.MaxIdleConns = o.MaxIdleConns
		tr.MaxIdleConnsPerHost = o.MaxIdleConnsPerHost
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: o.TLSServerName, InsecureSkipVerify: o.InsecureSkipVerify} // #nosec G402: opción explícita del administrador
		var transport http.RoundTripper = tr
		if o.Timeout > 0 {
			transport = &proxyTimeoutTransport{next: transport, timeout: o.Timeout}
		}
		if o.MaxConnections > 0 {
			transport = &proxyLimitTransport{next: transport, slots: make(chan struct{}, o.MaxConnections)}
		}
		proxy := newReverseProxy(name, target, transport)
		proxy.Rewrite = func(pr *httputil.ProxyRequest) {
			diagnostics.Emit("proxy", diagnostics.Trace, "HGW-PROXY-TRACE", "forward service=%q method=%s", name, pr.In.Method)
			originalHost := pr.In.Host
			pr.SetURL(target)
			if o.PreserveHost {
				pr.Out.Host = originalHost
			}
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Gateway-Service", name)
			for k, v := range o.RequestHeaders {
				pr.Out.Header.Set(k, v)
			}
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			level := diagnostics.Debug
			if resp.StatusCode >= 500 {
				level = diagnostics.Error
			} else if resp.StatusCode >= 400 {
				level = diagnostics.Warn
			}
			diagnostics.Emit("proxy", level, "HGW-PROXY-HTTP", "service=%q status=%d", name, resp.StatusCode)
			for k, v := range o.ResponseHeaders {
				resp.Header.Set(k, v)
			}
			return nil
		}
		if o.RedirectTo != "" {
			redirectURL := o.RedirectTo
			status := o.RedirectStatus
			proxy.Transport = &proxyRedirectTransport{target: redirectURL, status: status}
		}
		result[normalizeServiceName(name)+"."+domain] = proxy
		diagnostics.Emit("proxy", diagnostics.InfoLevel, "HGW-PROXY-006", "Proxy configurado: https://%s.%s -> %s", normalizeServiceName(name), domain, target.Redacted())
	}
	proxyTransportMu.Lock()
	activeProxyTransports = transports
	proxyTransportMu.Unlock()
	return result
}
