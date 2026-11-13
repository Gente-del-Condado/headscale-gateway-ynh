package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyAdministratorSessionIsBoundToEveryRequest(t *testing.T) {
	auth, err := NewAuthManager(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Update(AuthUpdate{Mode: "local", LocalUsername: "fallback", LocalPassword: "fallback-password-123"}); err != nil {
		t.Fatal(err)
	}
	s, _ := New(strings.Repeat("a", 64), &fakeController{})
	s.WithAuth(auth)
	options := HTTPOptions{BasePathHeader: "X-Ingress-Path", TrustedProxies: []string{"172.30.32.2"}, AllowFrame: true,
		ProxyAuth: &ProxyAuthOptions{UserHeader: "X-Remote-User-Id", GroupsHeader: "X-Remote-User-Groups", AdminGroup: "system-admin"}}
	h := s.Handler(options)
	request := func(handler http.Handler, method, path, address, user, groups, csrf, cookie, prefix, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://ha.test"+path, strings.NewReader(`{}`))
		r.RemoteAddr = address
		r.Header.Set("X-Ingress-Path", prefix)
		r.Header.Set("X-Remote-User-Id", user)
		r.Header.Set("X-Remote-User-Groups", groups)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Cookie", cookie)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	const proxy = "172.30.32.2:1234"
	const prefix = "/api/hassio_ingress/test"
	w := request(h, "GET", "/api/v1/auth/session", proxy, "alice", "system-users,system-admin", "", "", prefix, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session struct {
		User, Role, Source, CSRF string
		Authenticated            bool
	}
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if !session.Authenticated || session.User != "proxy:alice" || session.Role != "admin" || session.Source != "proxy" || len(session.CSRF) != 64 {
		t.Fatalf("invalid delegated session: %+v", session)
	}
	if len(w.Result().Cookies()) != 0 || len(auth.sessions) != 0 {
		t.Fatal("proxy identity minted a reusable session")
	}
	// Existing local cookies must not replace missing or non-admin HA identity.
	local := httptest.NewRecorder()
	auth.CreateSession(local, httptest.NewRequest("GET", "https://ha.test/", nil), "fallback")
	cookie := local.Result().Cookies()[0].String()
	for _, tc := range []struct {
		name, user, groups, address, csrf, mount, origin string
		want                                             int
	}{
		{"missing ID", "", "system-admin", proxy, "", prefix, "", 403},
		{"missing groups", "alice", "", proxy, "", prefix, "", 403},
		{"non admin", "alice", "system-users", proxy, "", prefix, "", 403},
		{"group substring", "alice", "not-system-admin", proxy, "", prefix, "", 403},
		{"wrong group case", "alice", "SYSTEM-ADMIN", proxy, "", prefix, "", 403},
		{"forged origin IP", "alice", "system-admin", "127.0.0.1:1234", "", prefix, "", 403},
		{"missing csrf", "alice", "system-admin", proxy, "", prefix, "https://ha.test", 401},
		{"wrong user csrf", "bob", "system-admin", proxy, session.CSRF, prefix, "https://ha.test", 401},
		{"wrong mount csrf", "alice", "system-admin", proxy, session.CSRF, "/api/hassio_ingress/other", "https://ha.test", 401},
		{"foreign browser origin", "alice", "system-admin", proxy, session.CSRF, prefix, "https://evil.test", 403},
		{"admin action", "alice", "system-admin", proxy, session.CSRF, prefix, "https://ha.test", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(h, "POST", "/api/v1/reload", tc.address, tc.user, tc.groups, tc.csrf, cookie, tc.mount, tc.origin)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	// Otro listener nunca confía en esas mismas cabeceras ni en el CSRF del proxy.
	w = request(s.Handler(), "GET", "/api/v1/auth/session", proxy, "alice", "system-admin", session.CSRF, "", prefix, "")
	if w.Code != 401 {
		t.Fatal("direct listener accepted a proxy identity")
	}
	w = request(h, "GET", "/api/v1/auth/methods", proxy, "alice", "system-admin", "", "", prefix, "")
	if !strings.Contains(w.Body.String(), `"mode":"proxy"`) {
		t.Fatal(w.Body.String())
	}
	for _, path := range []string{"/api/v1/auth/logout", "/api/v1/auth/mfa/setup"} {
		w = request(h, "POST", path, proxy, "alice", "system-admin", session.CSRF, "", prefix, "https://ha.test")
		if w.Code != 403 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if !auth.VerifyLocal("fallback", "fallback-password-123") {
		t.Fatal("proxy login changed fallback account")
	}
	for _, header := range []string{"X-Remote-User-Id", "X-Remote-User-Groups"} {
		r := httptest.NewRequest("GET", "https://ha.test/api/v1/auth/session", nil)
		r.RemoteAddr = proxy
		r.Header.Set("X-Ingress-Path", prefix)
		r.Header.Set("X-Remote-User-Id", "alice")
		r.Header.Set("X-Remote-User-Groups", "system-admin")
		r.Header.Add(header, "system-admin")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("ambiguous identity headers accepted", header, w.Code)
		}
	}
}

func TestProxyAuthenticationRejectsUnsafeConfiguration(t *testing.T) {
	valid := HTTPOptions{BasePathHeader: "X-Ingress-Path", TrustedProxies: []string{"172.30.32.2"},
		ProxyAuth: &ProxyAuthOptions{UserHeader: "X-Remote-User-Id", GroupsHeader: "X-Remote-User-Groups", AdminGroup: "system-admin"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*HTTPOptions){
		func(o *HTTPOptions) { o.TrustedProxies = nil },
		func(o *HTTPOptions) { o.BasePathHeader = "" },
		func(o *HTTPOptions) { o.ProxyAuth.AdminGroup = "" },
		func(o *HTTPOptions) { o.ProxyAuth.GroupsHeader = o.ProxyAuth.UserHeader },
	} {
		options := valid
		copyAuth := *valid.ProxyAuth
		options.ProxyAuth = &copyAuth
		modify(&options)
		if options.Validate() == nil {
			t.Fatal("unsafe proxy authentication accepted")
		}
	}
}
