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

func TestHTTPProxyTrustAndIsolation(t *testing.T) {
	s, _ := New(strings.Repeat("a", 64), &fakeController{})
	h := s.Handler(HTTPOptions{BasePathHeader: "X-Ingress-Path", TrustedProxies: []string{"172.30.32.2"}, AllowFrame: true})
	for _, tc := range []struct {
		address, prefix string
		want            int
	}{
		{"192.168.1.4:1234", "/api/hassio_ingress/abc", 403},
		{"127.0.0.1:1234", "/api/hassio_ingress/abc", 403},
		{"172.30.32.2:1234", "", 400},
		{"172.30.32.2:1234", "//evil.test", 400},
		{"172.30.32.2:1234", "/api/hassio_ingress/abc/../x", 400},
		{"172.30.32.2:1234", "/api/hassio_ingress/abc_-123", 200},
	} {
		r := httptest.NewRequest("GET", "http://ha.test/admin/", nil)
		r.RemoteAddr = tc.address
		r.Header.Set("X-Ingress-Path", tc.prefix)
		r.Header.Set("X-Forwarded-For", "172.30.32.2")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://ha.test/api/v1/status", nil)
	r.RemoteAddr = "172.30.32.2:1234"
	r.Header.Set("X-Ingress-Path", "/api/hassio_ingress/abc")
	r.Header.Set("X-Remote-User-Id", "admin")
	r.Header.Set("Remote-User", "admin")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("forwarded user headers authenticated a request")
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/admin/", nil))
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("standalone framing changed")
	}
}

func TestHTTPAuthenticationAcrossMounts(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "gateway.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, _ := NewAuthManagerSQLite(db)
	if err := a.Update(AuthUpdate{Mode: "local", LocalUsername: "admin", LocalPassword: "test-password-1234"}); err != nil {
		t.Fatal(err)
	}
	s, _ := New(strings.Repeat("a", 64), &fakeController{})
	s.WithAuth(a)
	if err := s.WithState(db); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name, prefix, address string
		options               HTTPOptions
	}{
		{"direct", "", "127.0.0.1:1234", HTTPOptions{}},
		{"fixed mount", "/gateway", "127.0.0.1:1234", HTTPOptions{BasePath: "/gateway", TrustedProxies: []string{"127.0.0.1"}, AllowFrame: true}},
		{"proxy header", "/api/hassio_ingress/abc", "172.30.32.2:1234", HTTPOptions{BasePathHeader: "X-Ingress-Path", TrustedProxies: []string{"172.30.32.2"}, AllowFrame: true}},
	} {
		prefix, address := fixture.prefix, fixture.address
		h := s.Handler(fixture.options)
		request := func(method, path, body, cookie, csrf, origin string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "http://ha.test"+path, strings.NewReader(body))
			r.RemoteAddr = address
			r.Header.Set("X-Ingress-Path", prefix)
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", origin)
			if cookie != "" {
				r.Header.Set("Cookie", cookie)
			}
			r.Header.Set("X-CSRF-Token", csrf)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		w := request("GET", "/", "", "", "", "")
		if w.Header().Get("Location") != prefix+"/admin/" {
			t.Fatal(w.Header())
		}
		w = request("POST", "/api/v1/auth/login", `{"username":"admin","password":"test-password-1234"}`, "", "", "https://ha.test")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Path != prefix+"/" || !cookies[0].Secure {
			t.Fatal(cookies)
		}
		var session struct {
			CSRF string `json:"csrf"`
		}
		json.Unmarshal(w.Body.Bytes(), &session)
		for _, tc := range []struct {
			csrf, origin string
			want         int
		}{{"", "https://ha.test", 401}, {session.CSRF, "https://evil.test", 403}, {session.CSRF, "https://ha.test", 200}} {
			w = request("POST", "/api/v1/reload", "{}", cookies[0].String(), tc.csrf, tc.origin)
			if w.Code != tc.want {
				t.Fatalf("%s %+v: %d %s", fixture.name, tc, w.Code, w.Body.String())
			}
		}
		w = request("GET", "/admin/", "", "", "", "")
		if w.Header().Get("X-Frame-Options") != map[bool]string{true: "SAMEORIGIN", false: "DENY"}[fixture.options.AllowFrame] || !strings.Contains(w.Body.String(), `src="http-paths.js"`) {
			t.Fatal("mount/framing")
		}
		w = request("GET", "/pki/v1/public-ca", "", "", "", "")
		if w.Code != http.StatusNotFound {
			t.Fatal("public endpoint exposed")
		}
	}
}

func TestSameRoutesAtDirectAndProxyMounts(t *testing.T) {
	s, _ := New(strings.Repeat("a", 64), &fakeController{})
	for _, prefix := range []string{"", "/gateway", "/nested/gateway"} {
		h := s.Handler(HTTPOptions{BasePath: prefix})
		for _, path := range []string{"/admin/", "/api/v1/status", "/admin/http-paths.js"} {
			r := httptest.NewRequest("GET", prefix+path, nil)
			r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("mount=%s route=%s: %d", prefix, path, w.Code)
			}
		}
	}
}

func TestRecoveryUsesSameHandlerWithExplicitProxyTrust(t *testing.T) {
	db, err := state.Open(filepath.Join(t.TempDir(), "gateway.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	a.SetBootstrap(token, "physical-console-code")
	s, _ := New(token, &fakeController{})
	s.WithAuth(a)
	request := func(h http.Handler) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/auth/bootstrap", strings.NewReader(`{"code":"physical-console-code"}`))
		r.RemoteAddr = "172.30.32.2:1234"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request(s.Handler()); w.Code != 403 {
		t.Fatal("untrusted remote redeemed recovery", w.Code)
	}
	h := s.Handler(HTTPOptions{TrustedProxies: []string{"172.30.32.2"}})
	if w := request(h); w.Code != 200 {
		t.Fatal("trusted recovery failed", w.Code, w.Body.String())
	}
	if w := request(h); w.Code != 403 {
		t.Fatal("recovery replay accepted", w.Code)
	}
}
