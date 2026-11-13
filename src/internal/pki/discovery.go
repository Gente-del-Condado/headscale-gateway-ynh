package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

// PeerURL solo admite HTTPS dentro de LAN o tailnet. Resuelve el nombre antes
// de conectar y fija la IP resuelta para evitar cambios de destino mediante DNS.
func PeerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("introduce solo el origen HTTPS del Gateway (sin rutas, credenciales ni parámetros)")
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, errors.New("puerto HTTPS fuera del intervalo 1..65535")
		}
	}
	return u, nil
}
func permittedIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if a.IsLoopback() || a.IsUnspecified() || a.IsMulticast() {
		return false
	}
	if a.IsPrivate() || a.IsLinkLocalUnicast() {
		return true
	}
	return netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}
func fetchIP(ctx context.Context, name string) (string, error) {
	ips, e := net.DefaultResolver.LookupIP(ctx, "ip", name)
	if e != nil {
		return "", e
	}
	if len(ips) == 0 {
		return "", errors.New("sin direcciones IP")
	}
	for _, ip := range ips {
		if !permittedIP(ip) {
			return "", errors.New("el peer debe resolverse exclusivamente en LAN o Tailnet")
		}
	}
	return ips[0].String(), nil
}

// Sync verifica el destino con la CA ya aprobada. Nunca acepta silenciosamente
// otra raíz ni establece confianza desde una URL sin verificar.
func Sync(ctx context.Context, a Anchor) (Anchor, error) {
	if !a.Approved {
		return a, errors.New("la CA debe aprobarse antes de sincronizar")
	}
	u, err := PeerURL(a.Endpoint)
	if err != nil {
		return a, err
	}
	original, err := ParseRoot([]byte(a.PEM))
	if err != nil {
		return a, err
	}
	// No utilizar las CA públicas del sistema: el peer debe demostrar que su
	// endpoint HTTPS está firmado por la CA APROBADA y no por cualquier CA.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(original.PEM)) {
		return a, errors.New("CA inválida")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ip, err := fetchIP(ctx, u.Hostname())
	if err != nil {
		return a, err
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: u.Hostname()}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(u.String(), "/")+"/pki/v1/public-ca", nil)
	if err != nil {
		return a, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return a, fmt.Errorf("no se puede verificar la conexión HTTPS del peer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return a, fmt.Errorf("el Gateway remoto respondió HTTP %d", resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxPEMSize+1))
	if err != nil {
		return a, err
	}
	received, err := ParseRoot(content)
	if err != nil {
		return a, err
	}
	if received.Fingerprint != original.Fingerprint {
		return a, errors.New("CA remota cambiada: nueva aprobación necesaria")
	}
	a.LastSync = time.Now().UTC()
	return a, nil
}

// ResolvePeerIP validates all DNS answers and pins one LAN/Tailnet address.
func ResolvePeerIP(ctx context.Context, name string) (string, error) { return fetchIP(ctx, name) }
