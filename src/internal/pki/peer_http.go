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
	"sort"
	"strings"
	"time"
)

type PeerDial func(context.Context, string, string) (net.Conn, error)
type Target struct {
	Origin  string `json:"origin"`
	IP      string `json:"ip"`
	Network string `json:"network"`
}
type Candidate struct {
	Name      string   `json:"name"`
	Endpoint  string   `json:"endpoint"`
	Addresses []string `json:"addresses"`
	Targets   []Target `json:"targets"`
	CATargets []Target `json:"ca_http_targets,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// HTTP solo se permite para un destino inicial de CA pública elegido explícitamente.
func publicCAOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	original := u.Scheme
	if original != "https" && original != "http" {
		return nil, errors.New("origen de CA inválido")
	}
	u.Scheme = "https"
	validated, err := PeerURL(u.String())
	if err != nil {
		return nil, err
	}
	validated.Scheme = original
	return validated, nil
}

func IsTailnetIP(raw string) bool {
	a, e := netip.ParseAddr(raw)
	return e == nil && (netip.MustParsePrefix("100.64.0.0/10").Contains(a.Unmap()) || netip.MustParsePrefix("fd7a:115c:a1e0::/48").Contains(a))
}

// Plan fija los destinos privados. Las pistas proceden de Headscale o de anuncios
// LAN; no conceden confianza y TLS sigue autenticando el nombre del servidor.
func Plan(ctx context.Context, origin string, hints []string) ([]Target, error) {
	u, err := PeerURL(origin)
	if err != nil {
		return nil, err
	}
	ips, dnsErr := net.DefaultResolver.LookupHost(ctx, u.Hostname())
	if dnsErr == nil {
		for _, ip := range ips {
			if !permittedIP(net.ParseIP(ip)) {
				return nil, errors.New("el peer debe resolverse exclusivamente en LAN o Tailnet")
			}
		}
	}
	ips = append(ips, hints...)
	return planIPs(u, ips)
}
func planIPs(u *url.URL, ips []string) ([]Target, error) {
	out := []Target{}
	seen := map[string]bool{}
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil || !permittedIP(ip) {
			continue
		}
		raw = ip.String()
		if seen[raw] {
			continue
		}
		seen[raw] = true
		v := *u
		v.Path = ""
		network := "LAN"
		port := u.Port()
		if port == "" {
			port = "8443"
		}
		if IsTailnetIP(raw) {
			network = "Tailnet"
			v.Host = u.Hostname()
		} else {
			v.Host = net.JoinHostPort(u.Hostname(), port)
		}
		out = append(out, Target{v.String(), raw, network})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Network == "Tailnet" && out[j].Network != "Tailnet" })
	if len(out) == 0 {
		return nil, errors.New("no hay direcciones LAN o Tailnet disponibles")
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out, nil
}

// GetPublic se limita a los dos recursos de federación. El acceso inicial solo
// admite CA pública: sin credenciales, cookies, redirecciones ni proxy. Aísla
// raíces desconocidas y valida nombre y vigencia antes de leer datos HTTP.
func GetPublic(ctx context.Context, target Target, path, ca string, bootstrap bool, dial PeerDial, limit int64) ([]byte, error) {
	return getPublicWithSystemDial(ctx, target, path, ca, bootstrap, dial, limit, nil)
}
func getPublicWithSystemDial(ctx context.Context, target Target, path, ca string, bootstrap bool, dial PeerDial, limit int64, systemDial PeerDial) ([]byte, error) {
	if path != "/pki/v1/public-ca" && path != "/.well-known/headscale-gateway" {
		return nil, errors.New("recurso de federación no permitido")
	}
	if bootstrap && path != "/pki/v1/public-ca" {
		return nil, errors.New("solo la CA pública admite descarga pendiente")
	}
	u, err := publicCAOrigin(target.Origin)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "http" && (!bootstrap || path != "/pki/v1/public-ca" || IsTailnetIP(target.IP)) {
		return nil, errors.New("HTTP solo se permite para descargar una CA pendiente por LAN")
	}
	ip := net.ParseIP(target.IP)
	if !permittedIP(ip) {
		return nil, errors.New("dirección de peer no permitida")
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	if IsTailnetIP(target.IP) {
		if port != "443" {
			return nil, errors.New("Tailnet requiere HTTPS 443")
		}
		if dial == nil {
			return nil, errors.New("VPN del Gateway no disponible")
		}
	} else {
		dial = systemDial
		if dial == nil {
			dial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
		}
	}
	roots, _ := x509.SystemCertPool()
	if roots == nil {
		roots = x509.NewCertPool()
	}
	if ca != "" && !roots.AppendCertsFromPEM([]byte(ca)) {
		return nil, errors.New("CA de federación inválida")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: u.Hostname()}
	if bootstrap {
		tlsConfig.InsecureSkipVerify = true // Quarantined bootstrap, NEVER used by catalogs or CA synchronization.
		tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("sin certificado HTTPS")
			}
			cert := cs.PeerCertificates[0]
			if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
				return errors.New("certificado HTTPS fuera de validez")
			}
			return cert.VerifyHostname(u.Hostname())
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	transport := &http.Transport{TLSClientConfig: tlsConfig, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dial(ctx, "tcp", net.JoinHostPort(target.IP, port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(target.Origin, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s %s: HTTP %d", target.Network, target.Origin, res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("respuesta de federación demasiado grande")
	}
	return raw, nil
}
func DownloadCA(ctx context.Context, targets []Target, dial PeerDial) (Anchor, error) {
	return downloadCAWith(ctx, targets, dial, GetPublic)
}

func downloadCAWith(ctx context.Context, targets []Target, dial PeerDial, get func(context.Context, Target, string, string, bool, PeerDial, int64) ([]byte, error)) (Anchor, error) {
	var failures []error
	for _, target := range targets {
		for _, admin := range []bool{true, false} {
			t := target
			u, _ := publicCAOrigin(t.Origin)
			if u == nil {
				continue
			}
			if u.Scheme == "http" && admin {
				continue
			}
			if admin {
				host := "admin." + u.Hostname()
				if u.Port() != "" {
					host = net.JoinHostPort(host, u.Port())
				}
				u.Host = host
				t.Origin = u.String()
			}
			raw, err := get(ctx, t, "/pki/v1/public-ca", "", true, dial, maxPEMSize)
			if err == nil {
				a, e := ParseRoot(raw)
				if e == nil {
					a.Endpoint = target.Origin
					a.Name = u.Hostname()
					a.Approved = false
					return a, nil
				}
				err = e
			}
			failures = append(failures, err)
			if ctx.Err() != nil {
				return Anchor{}, ctx.Err()
			}
		}
	}
	return Anchor{}, errors.Join(failures...)
}
