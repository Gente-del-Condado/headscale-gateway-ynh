package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReferenceMatchesRegisteredRoutes(t *testing.T) {
	key := strings.Repeat("a", 64)
	s, err := New(key, &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("/api/v1/reference", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request("/api/v1/reference", key)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Routes    []referenceEntry `json:"routes"`
		Commands  []any            `json:"commands"`
		Contracts map[string]any   `json:"contracts"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	mux := &referenceMux{ServeMux: http.NewServeMux()}
	s.registerStaticRoutes(mux)
	s.registerAPIRoutes(mux)
	s.registerReferenceRoutes(mux)
	if len(result.Routes) != len(mux.entries)+1 {
		t.Fatalf("missing registrations: %d/%d", len(result.Routes), len(mux.entries))
	}
	found := map[string]referenceEntry{}
	for _, entry := range result.Routes {
		key := entry.Method + " " + entry.Path
		if _, ok := found[key]; ok {
			t.Fatal("duplicate", key)
		}
		found[key] = entry
	}
	for _, key := range []string{"POST /api/v1/portal/sync", "POST /api/v1/portal/discover", "GET /api/v1/reference/peer/{fingerprint}", "GET /.well-known/headscale-gateway", "POST /api/v1/network/disconnect"} {
		if _, ok := found[key]; !ok {
			t.Fatal("missing", key)
		}
	}
	for _, key := range []string{"POST /api/v1/network/disconnect", "PUT /api/v1/network/gateway/preferences", "GET /api/v1/backups", "GET /api/v1/portal", "GET /api/v1/reference/peer/{fingerprint}"} {
		r := found[key]
		if r.Auth != "admin_session" || len(r.Roles) != 1 || r.Roles[0] != "admin" {
			t.Fatal("wrong session contract", key, r)
		}
	}
	r := found["GET /api/v1/network/admin/users"]
	if len(r.Roles) != 1 || r.Roles[0] != "admin" {
		t.Fatal(r)
	}
	if len(result.Commands) < 7 || len(result.Contracts) < 90 {
		t.Fatal("incomplete contracts")
	}
	if w := request("/api/v1/reference/peer/"+strings.Repeat("a", 64), key); w.Code != 403 {
		t.Fatal("peer accepts bearer", w.Code)
	}
	if path := os.Getenv("HGW_REFERENCE_EXPORT"); path != "" {
		if err = os.WriteFile(path, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
