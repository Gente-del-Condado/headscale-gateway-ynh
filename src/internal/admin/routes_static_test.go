package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecursosDelPanelYSeguridad(t *testing.T) {
	s, err := New(strings.Repeat("a", 64), &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler() // detecta también patrones duplicados y ServeMux inválido
	for path, mime := range map[string]string{
		"/admin/": "text/html", "/admin/panel.js": "text/javascript", "/admin/panel.css": "text/css",
		"/admin/panel-pki.js": "text/javascript", "/admin/panel-headscale.js": "text/javascript",
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), mime) {
			t.Errorf("recurso %s: code=%d mime=%s", path, w.Code, w.Header().Get("Content-Type"))
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Error("falta nosniff")
		}
	}
	for _, bad := range []string{"/admin/locales/noexiste.json", "/admin/locales/en.exe", "/admin/locales/en.json/otro", "/admin/ruta-inexistente", "/admin/panel.js/otro"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", bad, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("ruta inesperada %s: recibido HTTP %d, esperado 404", bad, w.Code)
		}
	}
}

func TestRecursosEmbebidosRegistrados(t *testing.T) {
	for urlPath, entry := range webAssets {
		if !strings.HasPrefix(urlPath, "/admin/") {
			t.Fatalf("ruta fuera de admin: %q", urlPath)
		}
		if _, err := assets.ReadFile(entry.file); err != nil {
			t.Errorf("no existe recurso %s: %v", entry.file, err)
		}
	}
}
