package main

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Listener aislado: nunca expone el mux administrativo en texto claro por LAN.
func publicCALANHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || pki.IsTailnetIP(host) {
			http.Error(w, i18n.Translate(i18n.Language(r.Header.Get("Accept-Language")), "Acceso exclusivo desde LAN"), http.StatusForbidden)
			return
		}
		if r.URL.Path != "/pki/v1/public-ca" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		clean := r.Clone(r.Context())
		clean.Header = make(http.Header)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, clean)
	})
}
func startPublicCALAN(ctx context.Context, cfg Config, runtime *gatewayRuntime, handler http.Handler) {
	if !cfg.CAHTTPEnabled {
		return
	}
	listener, err := net.Listen("tcp", cfg.CAHTTPListenAddr)
	if err != nil {
		diagnostics.Emit("admin", diagnostics.Warn, "HGW-CA-HTTP-LISTEN", "No se pudo abrir HTTP de CA en %s; administración sigue disponible: %v", cfg.CAHTTPListenAddr, err)
		return
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	runtime.caHTTPPort.Store(int64(n))
	server := &http.Server{Handler: publicCALANHandler(handler), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8 << 10, ErrorLog: diagnostics.HTTPErrorLogger("ca-http")}
	go func() { <-ctx.Done(); server.Close() }()
	go func() {
		defer runtime.caHTTPPort.Store(0)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			diagnostics.Emit("admin", diagnostics.Warn, "HGW-CA-HTTP-SERVE", "Servidor HTTP de CA detenido: %v", err)
		}
	}()
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-CA-HTTP-READY", "CA pública HTTP LAN disponible en %s; requiere publicación habilitada y aprobación manual del receptor", listener.Addr())
}
