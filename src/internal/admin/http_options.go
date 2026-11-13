package admin

// HTTPOptions configura el mismo manejador administrativo para acceso directo
// o proxy inverso, sin nombres de plataforma ni rutas específicas de cada una.
import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

type HTTPOptions struct {
	BasePath       string            `json:"base_path,omitempty"`
	BasePathHeader string            `json:"base_path_header,omitempty"`
	TrustedProxies []string          `json:"trusted_proxies,omitempty"`
	AllowFrame     bool              `json:"allow_frame,omitempty"`
	ProxyAuth      *ProxyAuthOptions `json:"proxy_auth,omitempty"`
	groupLookup    func(context.Context, string, string) bool
}

// ProxyAuthOptions delega autenticación en un proxy expresamente confiable.
// Exige tanto una identidad verificada como un grupo administrativo explícito.
type ProxyAuthOptions struct {
	UserHeader   string `json:"user_header"`
	GroupsHeader string `json:"groups_header"`
	AdminGroup   string `json:"admin_group"`
	UnixGroup    string `json:"unix_group,omitempty"`
	SharedSecret string `json:"shared_secret,omitempty"`
}

type httpRequestOptions struct {
	prefix       string
	allowFrame   bool
	trustedProxy bool
	proxyUser    string
}
type httpRequestOptionsKey struct{}

var basePathPattern = regexp.MustCompile(`^(?:/[A-Za-z0-9_-]+)*$`)
var proxyHeaderPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,62}$`)
var proxyUserPattern = regexp.MustCompile(`^[A-Za-z0-9@._:-]{1,128}$`)
var unixGroupPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,63}$`)

func proxyHasAdminGroup(value, expected string) bool {
	if len(value) > 4096 {
		return false
	}
	for _, group := range strings.Split(value, ",") {
		if strings.TrimSpace(group) == expected {
			return true
		}
	}
	return false
}

func (o HTTPOptions) Validate() error {
	if !basePathPattern.MatchString(o.BasePath) {
		return errors.New("invalid HTTP base path")
	}
	if o.BasePathHeader != "" && o.BasePathHeader != "X-Ingress-Path" {
		return errors.New("unsupported base path header")
	}
	if o.BasePathHeader != "" && o.BasePath != "" {
		return errors.New("choose a fixed base path or a proxy base path header")
	}
	if o.BasePathHeader != "" && len(o.TrustedProxies) == 0 {
		return errors.New("base path headers require explicit trusted proxies")
	}
	for _, address := range o.TrustedProxies {
		if _, err := netip.ParseAddr(address); err != nil {
			return errors.New("trusted proxies must be explicit IP addresses")
		}
	}
	if p := o.ProxyAuth; p != nil {
		headerGroups := proxyHeaderPattern.MatchString(p.GroupsHeader) && proxyUserPattern.MatchString(p.AdminGroup) && p.UnixGroup == ""
		unixGroups := unixGroupPattern.MatchString(p.UnixGroup) && p.GroupsHeader == "" && p.AdminGroup == "" && p.SharedSecret != ""
		if len(o.TrustedProxies) == 0 || (o.BasePathHeader == "" && p.SharedSecret == "") ||
			!proxyHeaderPattern.MatchString(p.UserHeader) || strings.EqualFold(p.UserHeader, p.GroupsHeader) ||
			(!headerGroups && !unixGroups) {
			return errors.New("proxy authentication requires a trusted path proxy, identity headers and an explicit admin group")
		}
		if p.SharedSecret != "" {
			if secret, err := hex.DecodeString(p.SharedSecret); err != nil || len(secret) != 32 {
				return errors.New("proxy shared secret must contain 32 random bytes encoded as hexadecimal")
			}
		}
		for _, address := range o.TrustedProxies {
			if netip.MustParseAddr(address).Unmap().IsLoopback() && p.SharedSecret == "" {
				return errors.New("proxy identity on loopback requires a shared secret")
			}
		}
	}
	return nil
}

func externalPath(r *http.Request, path string) string {
	if d, ok := r.Context().Value(httpRequestOptionsKey{}).(httpRequestOptions); ok {
		return d.prefix + path
	}
	return path
}

func (o HTTPOptions) wrap(next http.Handler) http.Handler {
	validation := o.Validate()
	return http.HandlerFunc(func(w http.ResponseWriter, incoming *http.Request) {
		if validation != nil {
			http.Error(w, "Invalid HTTP configuration", http.StatusServiceUnavailable)
			return
		}
		trusted := false
		if len(o.TrustedProxies) > 0 {
			address, err := netip.ParseAddrPort(incoming.RemoteAddr)
			if err == nil {
				for _, allowed := range o.TrustedProxies {
					if address.Addr().Unmap() == netip.MustParseAddr(allowed).Unmap() {
						trusted = true
						break
					}
				}
			}
			if !trusted {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		// Todas las entradas exigen un token CSRF válido y un Origin coincidente,
		// tanto en peticiones directas como reenviadas.
		if incoming.Method != http.MethodGet && incoming.Method != http.MethodHead && !sameOrigin(incoming) {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		prefix := o.BasePath
		if o.BasePathHeader != "" {
			prefix = strings.TrimSuffix(incoming.Header.Get(o.BasePathHeader), "/")
			if prefix == "" || !basePathPattern.MatchString(prefix) {
				http.Error(w, "Invalid proxy base path", http.StatusBadRequest)
				return
			}
		}
		proxyUser := ""
		if p := o.ProxyAuth; p != nil {
			ids := incoming.Header.Values(p.UserHeader)
			secretOK := p.SharedSecret == ""
			if p.SharedSecret != "" {
				values := incoming.Header.Values("X-Gateway-Proxy-Token")
				secretOK = len(values) == 1 && subtle.ConstantTimeCompare([]byte(values[0]), []byte(p.SharedSecret)) == 1
			}
			if !trusted || !secretOK || len(ids) != 1 || !proxyUserPattern.MatchString(ids[0]) {
				http.Error(w, "Authenticated proxy administrator required", http.StatusForbidden)
				return
			}
			admin := false
			if p.UnixGroup != "" {
				lookup := o.groupLookup
				if lookup == nil {
					lookup = unixGroupHasUser
				}
				admin = lookup(incoming.Context(), p.UnixGroup, ids[0])
			} else {
				groups := incoming.Header.Values(p.GroupsHeader)
				admin = len(groups) == 1 && proxyHasAdminGroup(groups[0], p.AdminGroup)
			}
			if !admin {
				http.Error(w, "Authenticated proxy administrator required", http.StatusForbidden)
				return
			}
			proxyUser = ids[0]
		}
		r := incoming.Clone(context.WithValue(incoming.Context(), httpRequestOptionsKey{}, httpRequestOptions{
			prefix: prefix, allowFrame: o.AllowFrame, trustedProxy: trusted, proxyUser: proxyUser,
		}))
		if prefix != "" && strings.HasPrefix(r.URL.Path, prefix+"/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
			r.URL.RawPath = ""
		} else if prefix != "" && !trusted {
			if r.URL.Path == "/" || r.URL.Path == "/admin" || r.URL.Path == "/admin/" || r.URL.Path == prefix {
				http.Redirect(w, r, externalPath(r, "/admin/"), http.StatusMovedPermanently)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		// Solo el proxy confiable configurado puede aportar identidad o esquema de conexión.
		if p := o.ProxyAuth; p != nil {
			r.Header.Del(p.UserHeader)
			r.Header.Del(p.GroupsHeader)
		}
		r.Header.Del("X-Gateway-Proxy-Token")
		r.Header.Del("X-Forwarded-Host")
		r.Header.Del("X-Remote-User-Id")
		r.Header.Del("Remote-User")
		if trusted && r.Header.Get("X-Forwarded-Proto") == "https" {
			r.TLS = &tls.ConnectionState{}
		}
		if r.URL.Path == "/admin" {
			http.Redirect(w, r, externalPath(r, "/admin/"), http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Plain HTTP requires loopback or an explicit list of trusted reverse proxies.
func ValidateHTTPListenAddress(addr string, options HTTPOptions) error {
	if err := options.Validate(); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("invalid HTTP listen address")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 0 || number > 65535 {
		return errors.New("invalid HTTP listen port")
	}
	if host == "localhost" && len(options.TrustedProxies) == 0 {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || (!ip.IsLoopback() && len(options.TrustedProxies) == 0) {
		return errors.New("non-loopback HTTP requires explicit trusted proxies")
	}
	return nil
}
