package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeController struct {
	saved Service
	calls int
}

func (*fakeController) Status(context.Context) (any, error) {
	return map[string]any{"vpn_connected": false}, nil
}
func (*fakeController) Services(context.Context) ([]Service, error) { return []Service{}, nil }
func (*fakeController) Templates(context.Context) ([]string, error) { return []string{"default"}, nil }
func (*fakeController) Settings(context.Context) (any, error) {
	return map[string]string{"dns.technitium.token": ""}, nil
}
func (*fakeController) ConfigSchema(context.Context) ([]ConfigField, error) {
	return []ConfigField{{Key: "certificates.mode", Kind: "select", Options: []ConfigOption{{Value: "auto", Label: "Automático"}}}}, nil
}
func (*fakeController) UpdateSettings(context.Context, map[string]string) error { return nil }
func (f *fakeController) SaveService(_ context.Context, s Service) error {
	f.calls++
	f.saved = s
	return nil
}
func (*fakeController) DeleteService(context.Context, string) error { return nil }
func (*fakeController) Reload(context.Context) error                { return nil }

func TestAdminAuthAndCRUD(t *testing.T) {
	f := &fakeController{}
	server, e := New(strings.Repeat("a", 64), f)
	if e != nil {
		t.Fatal(e)
	}
	handler := server.Handler()
	request := func(method, path, auth, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request("GET", "/api/v1/status", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", got)
	}
	if got := request("GET", "/api/v1/status", "invalid", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("token incorrecto: %d", got)
	}
	w := request("GET", "/api/v1/status", strings.Repeat("a", 64), "")
	if w.Code != 200 {
		t.Fatalf("GET status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "vpn_connected") {
		t.Fatal("status incompleto")
	}
	if got := request("GET", "/api/v1/config/schema", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("esquema sin autenticación: %d", got)
	}
	choices := request("GET", "/api/v1/config/schema", strings.Repeat("a", 64), "")
	if choices.Code != 200 || !strings.Contains(choices.Body.String(), "certificates.mode") {
		t.Fatalf("esquema: %d %s", choices.Code, choices.Body.String())
	}
	if got := request("GET", "/admin/", "", "").Code; got != 200 {
		t.Fatalf("panel inaccesible: %d", got)
	}
	if got := request("POST", "/api/v1/services", strings.Repeat("a", 64), `{"name":"demo","enabled":true,"backend":"http://127.0.0.1:8080","unknown":1}`).Code; got != 400 {
		t.Fatalf("se aceptó campo desconocido: %d", got)
	}
	w = request("POST", "/api/v1/services", strings.Repeat("a", 64), `{"name":"demo","enabled":true,"backend":"http://127.0.0.1:8080","proxy":{"preserve_host":true}}`)
	if w.Code != 200 {
		t.Fatalf("crear: %d %s", w.Code, w.Body.String())
	}
	if f.calls != 1 || f.saved.Name != "demo" {
		t.Fatal("no se guardó el servicio")
	}
	var proxy map[string]any
	if err := json.Unmarshal(f.saved.Proxy, &proxy); err != nil || proxy["preserve_host"] != true {
		t.Fatal("proxy no preservado", err)
	}
	if got := request("PUT", "/api/v1/services/otro", strings.Repeat("a", 64), `{"name":"demo"}`).Code; got != 400 {
		t.Fatalf("PUT con nombre diferente aceptado: %d", got)
	}
	if got := request("POST", "/api/v1/reload", strings.Repeat("a", 64), `{}`).Code; got != 200 {
		t.Fatalf("reload: %d", got)
	}
	if got := request("PUT", "/api/v1/config", strings.Repeat("a", 64), `{"changes":{"domain.base_domain":"example.test"}}`).Code; got != 200 {
		t.Fatalf("config: %d", got)
	}
}
func TestTokenStrengthAndSecurityHeaders(t *testing.T) {
	if _, e := New("short", &fakeController{}); e == nil {
		t.Fatal("token débil aceptado")
	}
	s, _ := New(strings.Repeat("b", 64), &fakeController{})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/admin/panel.js", nil))
	if w.Code != 200 {
		t.Fatalf("JS: %d", w.Code)
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("falta CSP")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("falta no-store")
	}
}

type fakeRestarter struct {
	fakeController
	restarts int
}

func (f *fakeRestarter) ScheduleRestart(context.Context) error {
	f.restarts++
	return nil
}
func TestRestartNeedsAuthenticationAndReturns202(t *testing.T) {
	f := &fakeRestarter{}
	server, err := New(strings.Repeat("a", 64), f)
	if err != nil {
		t.Fatal(err)
	}
	run := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/system/restart", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}
	if got := run("").Code; got != http.StatusUnauthorized {
		t.Fatalf("reinicio desprotegido: %d", got)
	}
	if got := run(strings.Repeat("a", 64)).Code; got != http.StatusAccepted {
		t.Fatalf("reinicio %d", got)
	}
	if f.restarts != 1 {
		t.Fatalf("reinicios=%d", f.restarts)
	}
}
