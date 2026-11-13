package admin

import (
	"context"
	"encoding/json"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type portalFixture struct {
	fakeController
	called bool
}

func (f *portalFixture) PKICandidates(context.Context) (any, error) {
	f.called = true
	return true, nil
}
func (f *portalFixture) PKIDownload(context.Context, string) (any, error) {
	f.called = true
	return true, nil
}
func (f *portalFixture) PortalProbe(context.Context, string) (any, error) {
	f.called = true
	return true, nil
}
func (f *portalFixture) PortalGet(context.Context) (any, error) {
	f.called = true
	return portal.Default(), nil
}
func (f *portalFixture) PortalSave(context.Context, portal.Config) (any, error) {
	f.called = true
	return true, nil
}
func (f *portalFixture) PortalSync(context.Context) (any, error) { f.called = true; return true, nil }
func (f *portalFixture) PortalDiscover(context.Context) (any, error) {
	f.called = true
	return true, nil
}
func TestPortalAdministrationRequiresSessionAndCSRF(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctl := &portalFixture{}
	key := strings.Repeat("b", 64)
	s, err := New(key, ctl)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WithState(db); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	s.WithAuth(auth)
	handler := s.Handler()
	for _, route := range []struct{ method, path string }{{"POST", "/api/v1/pki/discover/download"}, {"GET", "/api/v1/pki/discover"}, {"GET", "/api/v1/reference/peer/fixture"}, {"GET", "/api/v1/portal"}, {"PUT", "/api/v1/portal"}, {"POST", "/api/v1/portal/discover"}, {"POST", "/api/v1/portal/sync"}} {
		body, _ := json.Marshal(portal.Default())
		if route.path == "/api/v1/pki/discover/download" {
			body = []byte(`{}`)
		}
		call := func(cookie, csrf, bearer string) int {
			r := httptest.NewRequest(route.method, "http://localhost"+route.path, strings.NewReader(string(body)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Cookie", cookie)
			r.Header.Set("X-CSRF-Token", csrf)
			if bearer != "" {
				r.Header.Set("Authorization", "Bearer "+bearer)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w.Code
		}
		ctl.called = false
		if got := call("", "", key); got != 403 || ctl.called {
			t.Fatal("bearer accepted", got)
		}
		for _, role := range []string{"viewer", "admin"} {
			w := httptest.NewRecorder()
			csrf, err := auth.CreateSessionRoleRemember(w, httptest.NewRequest("GET", "http://localhost/admin/", nil), role, role, false)
			if err != nil {
				t.Fatal(err)
			}
			cookie := w.Result().Cookies()[0].String()
			if route.method != "GET" && call(cookie, "", "") == 200 {
				t.Fatal("CSRF bypass")
			}
			got := call(cookie, csrf, "")
			if role == "admin" && got != 200 || role == "viewer" && got == 200 {
				t.Fatal(role, got)
			}
		}
	}
}
