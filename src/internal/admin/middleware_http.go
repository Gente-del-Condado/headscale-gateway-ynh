package admin

// Cabeceras defensivas, idioma y trazabilidad de todas las peticiones.
import (
	"net/http"
	"strings"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
)

func (s *Server) wrapHTTP(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := s.requestCount.Add(1)
		writer := &languageWriter{ResponseWriter: w, language: i18n.Language(r.Header.Get("Accept-Language")), status: 200}
		w = writer
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		if opts, ok := r.Context().Value(httpRequestOptionsKey{}).(httpRequestOptions); ok && opts.allowFrame {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'self'; form-action 'self'")
		}
		mux.ServeHTTP(w, r)
		if (strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/v1/diagnostics/logs")) || strings.HasPrefix(r.URL.Path, "/admin/auth/") {
			level := diagnostics.Debug
			if writer.status >= 500 {
				level = diagnostics.Error
			} else if writer.status >= 400 {
				level = diagnostics.Warn
			} else if r.Method != "GET" && r.Method != "HEAD" {
				level = diagnostics.InfoLevel
			}
			diagnostics.Emit("admin", level, "HGW-HTTP-001", "request_id=%d method=%s path=%s status=%d duration_ms=%d", requestID, r.Method, r.URL.EscapedPath(), writer.status, time.Since(started).Milliseconds())
		}
	})
}
