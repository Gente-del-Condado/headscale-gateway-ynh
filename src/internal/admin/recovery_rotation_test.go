package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func TestRecoveryRotationRequiresInteractiveAdmin(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	original := strings.Repeat("a", 64)
	if err := db.PutJSON("recovery_token_v1", original); err != nil {
		t.Fatal(err)
	}
	server, err := New(original, &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.WithState(db); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	server.WithAuth(auth)
	handler := server.Handler()
	post := func(bearer, cookie, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/api/v1/auth/recovery/rotate", strings.NewReader(`{"confirm":"ROTAR"}`))
		r.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := post(original, "", ""); got.Code != 403 {
		t.Fatalf("bearer rotate: %d %s", got.Code, got.Body.String())
	}
	writer := httptest.NewRecorder()
	csrf, err := auth.CreateSessionRoleRemember(writer, httptest.NewRequest("GET", "http://localhost/admin/", nil), "admin@example.org", "admin", false)
	if err != nil {
		t.Fatal(err)
	}
	cookie := writer.Result().Cookies()[0]
	if got := post("", cookie.String(), ""); got.Code != 401 {
		t.Fatalf("CSRF missing: %d", got.Code)
	}
	got := post("", cookie.String(), csrf)
	if got.Code != 200 {
		t.Fatalf("rotate: %d %s", got.Code, got.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Token) != 64 || out.Token == original {
		t.Fatal("recovery key not rotated")
	}
	var dbToken string
	if ok, err := db.GetJSON("recovery_token_v1", &dbToken); err != nil || !ok || dbToken != out.Token {
		t.Fatalf("DB mismatch: %v", err)
	}
	request := func(token string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if status := request(original); status != 401 {
		t.Fatalf("old token active: %d", status)
	}
	if status := request(out.Token); status != 200 {
		t.Fatalf("new token rejected: %d", status)
	}
}

func TestDelegatedReloadScopeAndEmailRoles(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server, err := New(strings.Repeat("b", 64), &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.WithState(db); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	newKey := func(role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/api/tokens", strings.NewReader(`{"label":"reload","role":"`+role+`","scopes":["read","operations"],"days":30}`))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("b", 64))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := newKey("viewer"); got.Code != 400 {
		t.Fatalf("viewer reload allowed: %d", got.Code)
	}
	if got := newKey("operator"); got.Code != 201 {
		t.Fatalf("operator reload denied: %d %s", got.Code, got.Body.String())
	}
	if roleForOIDC(nil, map[string]string{"Admin@Example.ORG": "admin"}, nil, nil, "admin@example.org") != "admin" {
		t.Fatal("email role incorrect")
	}
	if roleForOIDC(nil, map[string]string{"Admin@Example.ORG": "admin"}, nil, nil, "other@example.org") != "" {
		t.Fatal("email mapping grants outsiders")
	}
	cfg := AuthConfig{Mode: "oidc", OIDC: OIDCSettings{Issuer: "https://accounts.google.com", ClientID: "client", ClientSecret: "secret", RedirectURL: "https://admin.example.org/admin/auth/oidc/callback", AllowedEmails: []string{"admin@example.org"}, DisableGroupsScope: true}, RoleEmails: map[string]string{"admin@example.org": "admin"}}
	if err := validateAuthConfig(cfg); err != nil {
		t.Fatalf("Google OIDC: %v", err)
	}
	cfg.RoleEmails = nil
	if err := validateAuthConfig(cfg); err == nil {
		t.Fatal("Google login without explicit role mapping")
	}
}
