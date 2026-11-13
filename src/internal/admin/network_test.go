package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeNetwork struct {
	fakeController
	disabled   bool
	pingCalled bool
}

func (f *fakeNetwork) NetworkInfo(context.Context) (any, error) {
	return map[string]any{"enabled": !f.disabled}, nil
}
func (f *fakeNetwork) NetworkNodes(context.Context) (any, error) {
	if f.disabled {
		return nil, ErrNetworkDisabled
	}
	return map[string]any{"nodes": []any{}}, nil
}
func (f *fakeNetwork) NetworkPing(_ context.Context, id, mode string) (any, error) {
	f.pingCalled = true
	return map[string]string{"id": id, "mode": mode}, nil
}
func (f *fakeNetwork) NetworkLogin(context.Context) (any, error) {
	return map[string]any{"state": "NeedsLogin"}, nil
}
func TestNetworkRoutesAndSecurity(t *testing.T) {
	f := &fakeNetwork{}
	server, err := New(strings.Repeat("Z", 64), f)
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler()
	call := func(method, path, body, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/v1/network/nodes", "", ""); w.Code != 401 {
		t.Fatalf("sin autorización: %d", w.Code)
	}
	key := strings.Repeat("Z", 64)
	if w := call("GET", "/api/v1/network/info", "", key); w.Code != 200 || !strings.Contains(w.Body.String(), "enabled") {
		t.Fatalf("info %d", w.Code)
	}
	if w := call("GET", "/api/v1/network/nodes", "", key); w.Code != 200 {
		t.Fatalf("lista: %d", w.Code)
	}
	if w := call("POST", "/api/v1/network/ping", `{"node_id":"1","mode":"disco","unknown":true}`, key); w.Code != 400 || f.pingCalled {
		t.Fatalf("JSON desconocido %d", w.Code)
	}
	if w := call("POST", "/api/v1/network/ping", `{"node_id":"1","mode":"disco"}`, key); w.Code != 200 || !f.pingCalled {
		t.Fatalf("ping: %d", w.Code)
	}
	if w := call("POST", "/api/v1/network/login", "", key); w.Code != 200 {
		t.Fatalf("login: %d", w.Code)
	}
	f.disabled = true
	if w := call("GET", "/api/v1/network/nodes", "", key); w.Code != http.StatusNotFound {
		t.Fatalf("panel desactivado no protege inventario: %d", w.Code)
	}
}
