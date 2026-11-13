package main

// Transporte HTTP de backends, límites y limpieza de conexiones.
import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// newHTTPTransport prepara un transporte reutilizable y limitado para los
// servicios de destino. No mantiene una caché de respuestas HTTP.
func newHTTPTransport() *http.Transport {
	return &http.Transport{

		// Permite utilizar las variables de entorno HTTP_PROXY,
		// HTTPS_PROXY y NO_PROXY para determinar si las conexiones
		// hacia los backends deben pasar por un proxy.
		Proxy: http.ProxyFromEnvironment,

		// Configuración del marcador utilizado para establecer
		// conexiones TCP hacia los backends.
		DialContext: (&net.Dialer{

			// Tiempo máximo permitido para establecer la conexión TCP.
			Timeout: 10 * time.Second,

			// Mantiene las conexiones TCP activas cuando son reutilizadas.
			KeepAlive: 30 * time.Second,
		}).DialContext,

		// Permite intentar HTTP/2 cuando el backend y la configuración
		// TLS lo permiten.
		ForceAttemptHTTP2: true,

		// Número máximo total de conexiones idle que puede mantener
		// este transporte para reutilizarlas.
		MaxIdleConns: 100,

		// Número máximo de conexiones idle mantenidas por cada backend.
		MaxIdleConnsPerHost: 20,

		// Tiempo máximo durante el que una conexión idle puede permanecer
		// abierta antes de cerrarse.
		IdleConnTimeout: 90 * time.Second,

		// Tiempo máximo permitido para completar el handshake TLS
		// cuando el backend utiliza HTTPS.
		TLSHandshakeTimeout: 10 * time.Second,

		// Tiempo máximo que se espera para recibir las cabeceras
		// de respuesta del backend.
		ResponseHeaderTimeout: 30 * time.Second,

		// Tiempo máximo de espera para continuar una petición que
		// utiliza el mecanismo HTTP "Expect: 100-continue".
		ExpectContinueTimeout: 1 * time.Second,
	}
}

type proxyTimeoutTransport struct {
	next    http.RoundTripper
	timeout time.Duration
}

func (t *proxyTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	resp, err := t.next.RoundTrip(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = proxyBodyOnClose(resp.Body, cancel)
	return resp, nil
}

type proxyCloseBody struct {
	io.ReadCloser
	done func()
	once sync.Once
}

func proxyBodyOnClose(body io.ReadCloser, done func()) io.ReadCloser {
	wrapped := &proxyCloseBody{ReadCloser: body, done: done}
	// En un 101, ReverseProxy escribe en el cuerpo para enviar datos al backend.
	if writer, ok := body.(io.Writer); ok {
		return struct {
			*proxyCloseBody
			io.Writer
		}{wrapped, writer}
	}
	return wrapped
}

// Close libera el límite de conexiones o cancela el timeout una sola vez,
// incluso cuando la biblioteca HTTP cierra el cuerpo más de una vez.
func (b *proxyCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.done)
	return err
}

type proxyLimitTransport struct {
	next  http.RoundTripper
	slots chan struct{}
}

func (t *proxyLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case t.slots <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		<-t.slots
		return nil, err
	}
	resp.Body = proxyBodyOnClose(resp.Body, func() { <-t.slots })
	return resp, nil
}

type proxyRedirectTransport struct {
	target string
	status int
}

func (t *proxyRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	location := t.target
	return &http.Response{StatusCode: t.status, Status: http.StatusText(t.status), Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

var proxyTransportMu sync.Mutex
var activeProxyTransports []*http.Transport

// ClearProxyConnections cierra solo conexiones TCP/TLS inactivas hacia el backend.
// No existe caché de respuestas: el Gateway no guarda en disco cookies,
// HTML ni respuestas de API privadas.
func clearIdleProxyConnections() {
	proxyTransportMu.Lock()
	defer proxyTransportMu.Unlock()
	for _, tr := range activeProxyTransports {
		tr.CloseIdleConnections()
	}
}
