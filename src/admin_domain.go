package main

// Publicación opcional del panel en HTTPS usando el certificado wildcard
// del Gateway. El listener HTTP de recuperación permanece en loopback.
import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"net"
	"net/http"
	"time"
)

type adminSiteHandler struct{ Handler http.Handler }
type ingressContextKey struct{}

func withGatewayIngress(next http.Handler, tailnet bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ingressContextKey{}, tailnet)))
	})
}

func newGatewayHTTPServer(runtime *gatewayRuntime, tailnet bool) *http.Server {
	return &http.Server{
		Handler:           withGatewayIngress(runtime, tailnet),
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, GetConfigForClient: runtime.GetConfigForClient},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      0, // soporte para WebSocket y servicios de streaming
		IdleTimeout:       120 * time.Second,
		ErrorLog:          diagnostics.HTTPErrorLogger("proxy"),
	}
}

// servePublishedAdmin solo admite peticiones cuyo Host coincida exactamente
// con el subdominio configurado. El ámbito del listener proviene del servidor
// que aceptó la conexión, nunca de X-Forwarded-For/Host/Proto.
func (r *gatewayRuntime) servePublishedAdmin(w http.ResponseWriter, req *http.Request, snap *gatewaySnapshot) bool {
	cfg := r.currentConfig()
	if !cfg.AdminEnabled || cfg.AdminDomainMode == "off" {
		return false
	}
	hostname := cfg.AdminSubdomain + "." + snap.domain
	if host := normalizeHost(req.Host); host != hostname && !localAdminHost(snap, host) {
		return false
	}
	if req.TLS == nil || (cfg.AdminDomainMode == "tailnet" && req.Context().Value(ingressContextKey{}) != true) {
		http.NotFound(w, req)
		return true
	}
	site := r.adminWeb.Load()
	if site == nil || site.Handler == nil {
		http.Error(w, "Administración no disponible", http.StatusServiceUnavailable)
		return true
	}
	// Este subdominio se dedica exclusivamente al panel, no al proxy general.
	// Se conserva la política CSP/CSRF/sesiones del handler administrativo.
	w.Header().Set("Strict-Transport-Security", "max-age=86400")
	site.Handler.ServeHTTP(w, req)
	return true
}

// El acceso por IP respeta la misma política HTTPS y de entrada que el dominio.
func localAdminHost(snap *gatewaySnapshot, host string) bool {
	if net.ParseIP(host) == nil && host != "localhost" {
		return false
	}
	if snap.certificates == nil {
		return false
	}
	certificate := snap.certificates.localTLSCertificate
	if certificate == nil {
		certificate = snap.certificates.directTLSCertificate
	}
	if certificate == nil {
		return false
	}
	if len(certificate.Certificate) == 0 {
		return false
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	return err == nil && leaf.VerifyHostname(host) == nil
}
