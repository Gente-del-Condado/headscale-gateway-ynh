package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnixGroupSSORequiresProxyProofAndCurrentMembership(t *testing.T) {
	auth, err := NewAuthManager(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := New(strings.Repeat("a", 64), &fakeController{})
	s.WithAuth(auth)
	member := true
	lookups := 0
	options := HTTPOptions{BasePath: "/gateway", TrustedProxies: []string{"127.0.0.1"},
		ProxyAuth: &ProxyAuthOptions{UserHeader: "Ynh-User", UnixGroup: "admins", SharedSecret: strings.Repeat("b", 64)},
		groupLookup: func(_ context.Context, group, user string) bool {
			lookups++
			return member && group == "admins" && user == "alice"
		}}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	h := s.Handler(options)
	request := func(user, secret, csrf, method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://ynh.test"+path, strings.NewReader(`{}`))
		r.RemoteAddr = "127.0.0.1:2345"
		r.Header.Set("Ynh-User", user)
		r.Header.Set("X-Gateway-Proxy-Token", secret)
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Origin", "https://ynh.test")
		r.Header.Set("X-Remote-User-Groups", "admins,system-admin")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, secret := range []string{"", "wrong"} {
		w := request("alice", secret, "", "GET", "/api/v1/auth/session")
		if w.Code != 403 {
			t.Fatal("local identity forgery accepted", w.Code)
		}
	}
	if lookups != 0 {
		t.Fatal("untrusted requests reached group lookup")
	}
	secret := options.ProxyAuth.SharedSecret
	w := request("alice", secret, "", "GET", "/api/v1/auth/session")
	if w.Code != 200 || len(w.Result().Cookies()) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	var session struct{ CSRF, Role, User string }
	json.Unmarshal(w.Body.Bytes(), &session)
	if session.Role != "admin" || session.User != "proxy:alice" {
		t.Fatal(session)
	}
	for _, tc := range []struct {
		csrf string
		want int
	}{{"", 401}, {session.CSRF, 200}} {
		if w := request("alice", secret, tc.csrf, "POST", "/api/v1/reload"); w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	member = false
	if w := request("alice", secret, session.CSRF, "GET", "/api/v1/auth/session"); w.Code != 403 {
		t.Fatal("removed administrator retained access")
	}
	if w := request("bob", secret, "", "GET", "/api/v1/auth/session"); w.Code != 403 {
		t.Fatal("forwarded groups granted administrator")
	}
	bad := options
	copyAuth := *options.ProxyAuth
	bad.ProxyAuth = &copyAuth
	bad.ProxyAuth.SharedSecret = ""
	if bad.Validate() == nil {
		t.Fatal("loopback delegation did not require private proxy proof")
	}
	bad.TrustedProxies = []string{"::ffff:127.0.0.1"}
	bad.BasePath = ""
	bad.BasePathHeader = "X-Ingress-Path"
	bad.ProxyAuth = &ProxyAuthOptions{UserHeader: "X-Remote-User-Id", GroupsHeader: "X-Remote-User-Groups", AdminGroup: "system-admin"}
	if bad.Validate() == nil {
		t.Fatal("mapped loopback bypassed the private proxy proof requirement")
	}
}

func TestSystemGroupRecordsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		record string
		want   bool
	}{
		{"admins:*:4001:alice,bob\n", true},
		{"admins:*:4001:malice", false},
		{"users:*:4001:alice", false},
		{"admins:*:4001:", false},
		{"admins:*:4001:alice\nadmins:*:4001:bob", false},
		{"invalid", false},
	} {
		if got := groupRecordHasUser(tc.record, "admins", "alice"); got != tc.want {
			t.Fatalf("%q: %v", tc.record, got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if unixGroupHasUser(ctx, "admins", "alice") {
		t.Fatal("failed/cancelled group lookup allowed access")
	}
	var output boundedGroupOutput
	if _, err := output.Write(make([]byte, (64<<10)+1)); err == nil {
		t.Fatal("unbounded group output")
	}
	if _, err := io.Copy(&output, io.LimitReader(strings.NewReader(strings.Repeat("a", (64<<10)+1)), (64<<10)+1)); err == nil {
		t.Fatal("stream copy bypassed group output limit")
	}
}
