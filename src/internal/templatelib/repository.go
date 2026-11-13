// El paquete templatelib define el formato JSON portable y versionado de plantillas.
// Un repositorio es una URL HTTPS directa que sirve ese JSON, nunca un ejecutable.
package templatelib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const Format = "headscale-gateway-templates"
const MaxBytes = 8 << 20
const MaxTemplates = 256

type Bundle[T any] struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	Templates []T    `json:"templates"`
}

func Decode[T any](data []byte) (Bundle[T], error) {
	var b Bundle[T]
	if len(data) > MaxBytes {
		return b, errors.New("archivo de plantillas demasiado grande")
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("JSON inválido: %w", err)
	}
	if b.Format != Format || b.Version != 1 || len(b.Templates) == 0 || len(b.Templates) > MaxTemplates {
		return b, errors.New("formato, versión o número de plantillas no admitido")
	}
	return b, nil
}

var blockedRanges = func() []netip.Prefix {
	prefixes := []string{
		"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24",
		"192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"64:ff9b::/96", "2001:db8::/32", "2001::/32",
	}
	out := make([]netip.Prefix, 0, len(prefixes))
	for _, value := range prefixes {
		out = append(out, netip.MustParsePrefix(value))
	}
	return out
}()

func safeIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range blockedRanges {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}
func permittedIP(ip net.IP, allowPrivate bool) bool {
	// Ni los repositorios privados confiables pueden acceder a metadatos link-local.
	if ip == nil || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return allowPrivate || safeIP(ip)
}

// validPort permits custom HTTPS ports for self-hosted Forgejo instances.
func validPort(v string) bool {
	n, e := strconv.Atoi(v)
	return e == nil && n >= 1 && n <= 65535
}

// Fetch bloquea redes privadas salvo autorización y evita cambios de DNS al
// conectar a la IP resuelta y validada. Nunca ejecuta contenido remoto.
func ValidateURL(rawURL string) error {
	if len(rawURL) > 2048 {
		return errors.New("URL del repositorio demasiado larga")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Port() != "" && !validPort(u.Port()) {
		return errors.New("el repositorio debe ser una URL HTTPS válida, sin credenciales ni fragmentos")
	}
	return nil
}

func Fetch(ctx context.Context, rawURL string, allowPrivate bool) ([]byte, error) {
	if err := ValidateURL(rawURL); err != nil {
		return nil, err
	}
	u, _ := url.Parse(rawURL)
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	resolver := net.DefaultResolver
	transport := &http.Transport{
		Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, e := net.SplitHostPort(addr)
			if e != nil {
				return nil, e
			}
			ips, e := resolver.LookupIP(ctx, "ip", host)
			if e != nil {
				return nil, e
			}
			var lastError error
			for _, ip := range ips {
				if !permittedIP(ip, allowPrivate) {
					continue
				}
				conn, e := (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if e == nil {
					return conn, nil
				}
				lastError = e
			}
			if lastError != nil {
				return nil, lastError
			}
			return nil, errors.New("el repositorio resuelve a una dirección no permitida; active acceso privado solo si confía en ese servidor")
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("no se permiten redirecciones de repositorio")
	}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	res, err := client.Do(request)
	if err != nil {
		// Los errores de http.Client pueden contener la URL completa y tokens privados.
		// No propaga esa URL a errores de API ni a registros persistentes.
		var requestError *url.Error
		if errors.As(err, &requestError) {
			return nil, fmt.Errorf("petición HTTPS al repositorio: %w", requestError.Err)
		}
		return nil, errors.New("error de conexión con el repositorio")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d al consultar el repositorio", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") && !strings.Contains(ct, "octet-stream") && !strings.Contains(ct, "text/plain") {
		return nil, errors.New("el repositorio no devuelve JSON")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, errors.New("respuesta del repositorio demasiado grande")
	}
	return data, nil
}
