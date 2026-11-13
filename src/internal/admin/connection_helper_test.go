package admin

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type disconnectFixture struct {
	fakeController
	called bool
}

func (f *disconnectFixture) NetworkDisconnect(context.Context) (any, error) {
	f.called = true
	return map[string]bool{"disconnected": true}, nil
}
func TestDisconnectAndHelperRequireInteractiveAdmin(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &disconnectFixture{}
	key := strings.Repeat("a", 64)
	s, err := New(key, f)
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
	h := s.Handler()
	for _, route := range []struct{ method, path, body string }{{"POST", "/api/v1/network/disconnect", "{}"}, {"PUT", "/api/v1/pki/helper", `{"enabled":false}`}} {
		call := func(cookie, csrf, bearer string) int {
			r := httptest.NewRequest(route.method, "http://localhost"+route.path, strings.NewReader(route.body))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Cookie", cookie)
			r.Header.Set("X-CSRF-Token", csrf)
			if bearer != "" {
				r.Header.Set("Authorization", "Bearer "+bearer)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w.Code
		}
		if code := call("", "", key); code != 403 {
			t.Fatalf("bearer %s: %d", route.path, code)
		}
		for _, role := range []string{"viewer", "admin"} {
			w := httptest.NewRecorder()
			csrf, err := auth.CreateSessionRoleRemember(w, httptest.NewRequest("GET", "http://localhost/admin/", nil), role, role, false)
			if err != nil {
				t.Fatal(err)
			}
			cookie := w.Result().Cookies()[0].String()
			if code := call(cookie, "", ""); code == 200 {
				t.Fatal("missing CSRF accepted")
			}
			code := call(cookie, csrf, "")
			if role == "admin" && code != 200 || role == "viewer" && code == 200 {
				t.Fatalf("role %s route %s: %d", role, route.path, code)
			}
		}
	}
	if !f.called {
		t.Fatal("disconnect not called")
	}
	if s.helperEnabled() {
		t.Fatal("helper should be disabled")
	}
}
