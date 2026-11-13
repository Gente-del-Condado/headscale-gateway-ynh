package portal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type Dial func(context.Context, string, string) (net.Conn, error)

func Fetch(ctx context.Context, origin, ca string, tailnet Dial) (Catalog, error) {
	return fetchResolved(ctx, origin, ca, tailnet, pki.ResolvePeerIP)
}
func fetchResolved(ctx context.Context, origin, ca string, tailnet Dial, resolve func(context.Context, string) (string, error)) (Catalog, error) {
	var result Catalog
	u, err := pki.PeerURL(origin)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	ip, err := resolve(ctx, u.Hostname())
	if err != nil {
		return result, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if ca != "" && !roots.AppendCertsFromPEM([]byte(ca)) {
		return result, errors.New("CA de federación inválida")
	}
	if pki.IsTailnetIP(ip) {
		u.Host = u.Hostname()
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	addr, _ := netip.ParseAddr(ip)
	dial := Dial((&net.Dialer{Timeout: 4 * time.Second}).DialContext)
	if netip.MustParsePrefix("100.64.0.0/10").Contains(addr) || netip.MustParsePrefix("fd7a:115c:a1e0::/48").Contains(addr) {
		if tailnet == nil {
			return result, errors.New("VPN del Gateway no disponible")
		}
		dial = tailnet
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: u.Hostname()}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dial(ctx, "tcp", net.JoinHostPort(ip, port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	origin = strings.TrimSuffix(u.String(), "/")
	req, err := http.NewRequestWithContext(ctx, "GET", origin+CatalogPath, nil)
	if err != nil {
		return result, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return result, errors.New("el portal remoto no publica un catálogo accesible")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil {
		return result, err
	}
	if len(raw) > 1024*1024 {
		return result, errors.New("catálogo remoto demasiado grande")
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, result.Validate(origin)
}

// FetchTarget reutiliza el plan de red con destino fijado y mantiene la validación del catálogo.
func FetchTarget(ctx context.Context, target pki.Target, ca string, tailnet Dial) (Catalog, error) {
	var result Catalog
	raw, err := pki.GetPublic(ctx, target, CatalogPath, ca, false, pki.PeerDial(tailnet), 1024*1024)
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, result.Validate(target.Origin)
}
