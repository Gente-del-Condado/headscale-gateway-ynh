package admin

// Recursos estáticos permitidos explícitamente. Nunca se forman rutas
// a partir de nombres de archivos proporcionados por el navegador.
import (
	"embed"
	"net/http"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
)

//go:embed http-paths.js panel.html panel.css panel.js panel-security.js panel-templates.js panel-experience.js panel-headscale.js panel-policy-dns.js panel-policy-visual.js panel-ops.js panel-backups.js panel-forgejo.js panel-pki.js panel-portability.js panel-portal.js panel-reference.js i18n.js proxy-model.js
var assets embed.FS

type webAsset struct{ file, mime string }

var webAssets = map[string]webAsset{
	"/admin/http-paths.js":          {"http-paths.js", "text/javascript; charset=utf-8"},
	"/admin/panel-reference.js":     {"panel-reference.js", "text/javascript; charset=utf-8"},
	"/admin/panel-portal.js":        {"panel-portal.js", "text/javascript; charset=utf-8"},
	"/admin/panel.html":             {"panel.html", "text/html; charset=utf-8"},
	"/admin/panel.css":              {"panel.css", "text/css; charset=utf-8"},
	"/admin/proxy-model.js":         {"proxy-model.js", "text/javascript; charset=utf-8"},
	"/admin/panel.js":               {"panel.js", "text/javascript; charset=utf-8"},
	"/admin/panel-headscale.js":     {"panel-headscale.js", "text/javascript; charset=utf-8"},
	"/admin/panel-policy-dns.js":    {"panel-policy-dns.js", "text/javascript; charset=utf-8"},
	"/admin/panel-policy-visual.js": {"panel-policy-visual.js", "text/javascript; charset=utf-8"},
	"/admin/panel-security.js":      {"panel-security.js", "text/javascript; charset=utf-8"},
	"/admin/panel-experience.js":    {"panel-experience.js", "text/javascript; charset=utf-8"},
	"/admin/panel-templates.js":     {"panel-templates.js", "text/javascript; charset=utf-8"},
	"/admin/panel-backups.js":       {"panel-backups.js", "text/javascript; charset=utf-8"},
	"/admin/panel-forgejo.js":       {"panel-forgejo.js", "text/javascript; charset=utf-8"},
	"/admin/panel-portability.js":   {"panel-portability.js", "text/javascript; charset=utf-8"},
	"/admin/panel-pki.js":           {"panel-pki.js", "text/javascript; charset=utf-8"},
	"/admin/panel-ops.js":           {"panel-ops.js", "text/javascript; charset=utf-8"},
	"/admin/i18n.js":                {"i18n.js", "text/javascript; charset=utf-8"},
}

func (s *Server) registerStaticRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, externalPath(r, "/admin/"), http.StatusFound)
	})
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, r *http.Request) {
		// En ServeMux, /admin/ también actúa como ruta comodín para cualquier
		// subruta que no coincida con otra regla explícita. No debemos devolver
		// el HTML del panel cuando se solicita una URL inexistente.
		if r.URL.Path != "/admin/" {
			http.NotFound(w, r)
			return
		}
		s.asset(w, r, "panel.html", "text/html; charset=utf-8")
	})
	for path, resource := range webAssets {
		resource := resource
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			s.asset(w, r, resource.file, resource.mime)
		})
	}
	mux.HandleFunc("GET /admin/locales/levels", func(w http.ResponseWriter, r *http.Request) {
		labels := map[string]string{}
		for _, lang := range i18n.Codes() {
			for source, code := range map[string]string{"ERROR": "error", "ADVERTENCIA": "warn", "INFORMACIÓN": "info", "DEPURACIÓN": "debug", "TRAZA": "trace"} {
				labels[i18n.Translate(lang, source)] = code
			}
		}
		reply(w, http.StatusOK, labels, nil)
	})
	mux.HandleFunc("GET /admin/locales", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, i18n.Codes(), nil)
	})
	mux.HandleFunc("GET /admin/locales/{filename}", func(w http.ResponseWriter, r *http.Request) {
		lang, ok := strings.CutSuffix(r.PathValue("filename"), ".json")
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := i18n.Data(lang)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
	})
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request, name, mime string) {
	data, e := assets.ReadFile(name)
	if e != nil {
		problem(w, 500, "asset no disponible")
		return
	}
	w.Header().Set("Content-Type", mime)
	_, _ = w.Write(data)
}
