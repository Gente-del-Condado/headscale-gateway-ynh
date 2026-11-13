package admin

import (
	"encoding/json"
	"github.com/gentedelcondado/headscale-gateway/internal/platformfs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalAuthConfigAndSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	a, err := NewAuthManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Update(AuthUpdate{Mode: "local", LocalUsername: "admin", LocalPassword: "very-long-test-password"}); err != nil {
		t.Fatal(err)
	}
	if !a.VerifyLocal("admin", "very-long-test-password") || a.VerifyLocal("admin", "incorrect") {
		t.Fatal("verificación local")
	}
	info, err := os.Stat(path)
	if err != nil || !platformfs.PrivateFile(info) {
		t.Fatalf("permisos auth.json: %v %v", info, err)
	}
	publicJSON, _ := json.Marshal(a.Public())
	if strings.Contains(string(publicJSON), "very-long-test-password") || strings.Contains(string(publicJSON), "$2a$") {
		t.Fatal("se filtró la contraseña")
	}
	req := httptest.NewRequest("GET", "http://127.0.0.1:9091/admin/", nil)
	rec := httptest.NewRecorder()
	csrf := a.CreateSession(rec, req, "admin")
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].HttpOnly {
		t.Fatal("falta cookie segura HttpOnly")
	}
	post := httptest.NewRequest("POST", "http://127.0.0.1:9091/api/v1/services", nil)
	post.AddCookie(cookies[0])
	if a.sessionAllowed(post) {
		t.Fatal("POST sin CSRF admitido")
	}
	post.Header.Set("X-CSRF-Token", csrf)
	if !a.sessionAllowed(post) {
		t.Fatal("POST con CSRF denegado")
	}
	a.Logout(httptest.NewRecorder(), post)
	if a.sessionAllowed(post) {
		t.Fatal("logout no revocó sesión")
	}
}
func TestBootstrapOneUseAndLoopback(t *testing.T) {
	a, err := NewAuthManager(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	a.SetBootstrap(strings.Repeat("a", 64), "one-time-code")
	remote := httptest.NewRequest("POST", "http://example.test/api/v1/auth/bootstrap", nil)
	remote.RemoteAddr = "192.168.1.2:12345"
	if _, err := a.RedeemBootstrap("one-time-code", remote); err == nil {
		t.Fatal("bootstrap remoto permitido")
	}
	local := httptest.NewRequest("POST", "http://127.0.0.1:9091/api/v1/auth/bootstrap", nil)
	local.RemoteAddr = "127.0.0.1:45678"
	if _, err := a.RedeemBootstrap("invalid", local); err == nil {
		t.Fatal("código incorrecto admitido")
	}
	got, err := a.RedeemBootstrap("one-time-code", local)
	if err != nil || got != strings.Repeat("a", 64) {
		t.Fatalf("código válido: %q %v", got, err)
	}
	if _, err := a.RedeemBootstrap("one-time-code", local); err == nil {
		t.Fatal("bootstrap reutilizado")
	}
}
func TestAuthModesRequireExplicitAllowlist(t *testing.T) {
	cases := []AuthConfig{
		{Mode: "ldap", LDAP: LDAPSettings{URL: "ldap://localhost:389", BaseDN: "dc=example,dc=org", AllowedUsers: []string{"admin"}}},
		{Mode: "ldap", LDAP: LDAPSettings{URL: "ldaps://localhost:636", BaseDN: "dc=example,dc=org"}},
		{Mode: "oidc", OIDC: OIDCSettings{Issuer: "http://dex.invalid", ClientID: "test", ClientSecret: "secret", RedirectURL: "https://admin.example/admin/auth/oidc/callback", AllowedEmails: []string{"root@example.org"}}},
		{Mode: "oidc", OIDC: OIDCSettings{Issuer: "https://dex.example/dex", ClientID: "test", ClientSecret: "secret", RedirectURL: "https://admin.example/admin/auth/oidc/callback"}},
	}
	for _, c := range cases {
		if validateAuthConfig(c) == nil {
			t.Fatalf("se aceptó configuración insegura: %s", c.Mode)
		}
	}
}
func TestLoginAPIAndCookieCSRF(t *testing.T) {
	a, _ := NewAuthManager(filepath.Join(t.TempDir(), "auth.json"))
	if err := a.Update(AuthUpdate{Mode: "local", LocalUsername: "admin", LocalPassword: "strong-password-123"}); err != nil {
		t.Fatal(err)
	}
	server, _ := New(strings.Repeat("b", 64), &fakeController{})
	server.WithAuth(a)
	handler := server.Handler()
	req := httptest.NewRequest("POST", "http://127.0.0.1:9091/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"strong-password-123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %d: %s", rec.Code, rec.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]
	status := httptest.NewRequest("GET", "http://127.0.0.1:9091/api/v1/status", nil)
	status.AddCookie(cookie)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, status)
	if out.Code != 200 {
		t.Fatal("sesión no permite lectura")
	}
	post := httptest.NewRequest("POST", "http://127.0.0.1:9091/api/v1/reload", strings.NewReader("{}"))
	post.AddCookie(cookie)
	post.Header.Set("Content-Type", "application/json")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, post)
	if out.Code != 401 {
		t.Fatal("POST sin CSRF debe denegarse")
	}
	post.Header.Set("X-CSRF-Token", data["csrf"].(string))
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, post)
	if out.Code != 200 {
		t.Fatalf("POST con CSRF: %d", out.Code)
	}
}
