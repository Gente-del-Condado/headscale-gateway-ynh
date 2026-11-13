package admin

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func TestRememberSessionPersistsAndRevokes(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "sessions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://127.0.0.1/admin/auth/login", nil)
	csrf, err := first.CreateSessionRoleRemember(response, req, "operator", "viewer", true)
	if err != nil || len(csrf) < 32 {
		t.Fatalf("persisted session: %v", err)
	}
	cookie := response.Result().Cookies()
	if len(cookie) == 0 || !cookie[0].HttpOnly || cookie[0].MaxAge != 30*24*3600 {
		t.Fatal("wrong cookie attributes")
	}
	second, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	access := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/auth/session", nil)
	access.AddCookie(cookie[0])
	s, ok := second.Session(access)
	if !ok || s.user != "operator" || s.role != "viewer" || s.csrf != csrf {
		t.Fatalf("not restored: %v %+v", ok, s)
	}
	second.Logout(httptest.NewRecorder(), access)
	third, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := third.Session(access); ok {
		t.Fatal("logout did not revoke persisted cookie")
	}
}

func TestOrdinarySessionIsNotRestored(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "sessions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://127.0.0.1/admin/auth/login", nil)
	if _, err = first.CreateSessionRoleRemember(response, req, "user", "viewer", false); err != nil {
		t.Fatal(err)
	}
	second, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	check := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/auth/session", nil)
	check.AddCookie(response.Result().Cookies()[0])
	if _, ok := second.Session(check); ok {
		t.Fatal("ordinary session was stored persistently")
	}
}

func TestMFAVisibleOnlyWhenLocalAndEnabled(t *testing.T) {
	a := emptyAuthManager("")
	if a.Methods()["mfa_enabled"].(bool) {
		t.Fatal("MFA must default to hidden")
	}
	a.mu.Lock()
	a.config.Mode = "local"
	a.config.MFAEnabled = true
	a.mu.Unlock()
	if !a.Methods()["mfa_enabled"].(bool) {
		t.Fatal("local MFA not exposed")
	}
	a.mu.Lock()
	a.config.Mode = "oidc"
	a.mu.Unlock()
	if a.Methods()["mfa_enabled"].(bool) {
		t.Fatal("local TOTP shown for OIDC")
	}
}
