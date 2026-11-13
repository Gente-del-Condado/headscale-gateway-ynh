package admin

import (
	"encoding/json"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRBAC(t *testing.T) {
	if !rolePermits("viewer", "network", "GET") || rolePermits("viewer", "settings", "GET") || rolePermits("viewer", "services", "PUT") {
		t.Fatal("viewer mal autorizado")
	}
	if !rolePermits("operator", "services", "POST") || rolePermits("operator", "settings", "PUT") || rolePermits("operator", "identity", "GET") {
		t.Fatal("operator mal autorizado")
	}
	if roleForGroups(map[string]string{"Admins": "admin", "Guests": "viewer"}, []string{"guests"}, nil) != "viewer" {
		t.Fatal("mapeo de grupos")
	}
	if roleForGroups(map[string]string{"Admins": "admin"}, []string{"other"}, nil) != "" {
		t.Fatal("grupo desconocido debe denegarse")
	}
}
func TestDelegatedAPIAndAudit(t *testing.T) {
	db, e := state.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if e != nil {
		t.Skip(e)
	}
	defer db.Close()
	f := &fakeController{}
	s, e := New(strings.Repeat("z", 64), f)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.WithState(db); e != nil {
		t.Fatal(e)
	}
	h := s.Handler()
	request := func(method, path, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	master := strings.Repeat("z", 64)
	created := request("POST", "/api/v1/api/tokens", master, `{"label":"Home Assistant","role":"viewer","scopes":["read"],"days":30}`)
	if created.Code != 201 {
		t.Fatalf("crear: %d %s", created.Code, created.Body.String())
	}
	var output struct {
		Token string `json:"token"`
	}
	if e = json.Unmarshal(created.Body.Bytes(), &output); e != nil {
		t.Fatal(e)
	}
	if got := request("GET", "/api/v1/status", output.Token, "").Code; got != 200 {
		t.Fatalf("lectura HA: %d", got)
	}
	if got := request("PUT", "/api/v1/config", output.Token, `{"changes":{"x":"1"}}`).Code; got != 403 {
		t.Fatalf("viewer editó ajustes: %d", got)
	}
	if got := request("GET", "/api/v1/audit", output.Token, "").Code; got != 403 {
		t.Fatalf("viewer leyó auditoría: %d", got)
	}
	a, err := NewAuthManager(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Update(AuthUpdate{Mode: "local", LocalUsername: "admin", LocalPassword: "correct-horse-battery-staple"}); err != nil {
		t.Fatal(err)
	}
	s.WithAuth(a)
	config := request("PUT", "/api/v1/api/exposure", master, `{"token_read":false,"token_services":true,"token_settings":true,"token_network":true,"token_operations":true,"token_identity":true}`)
	if config.Code != 200 {
		t.Fatalf("policy: %d", config.Code)
	}
	if got := request("GET", "/api/v1/status", output.Token, "").Code; got != 403 {
		t.Fatalf("política no bloquea: %d", got)
	}
	events, e := db.ListAudit(10)
	if e != nil || len(events) < 2 {
		t.Fatalf("auditoría: %+v %v", events, e)
	}
	if got := request("POST", "/api/v1/api/tokens", master, `{"label":"evil","role":"admin","scopes":["identity"],"days":1}`).Code; got != 400 {
		t.Fatalf("emitió rol admin: %d", got)
	}
}

var _ = http.StatusOK
