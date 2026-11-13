//go:build commercial

package main

import (
	"crypto/tls"
	"github.com/gentedelcondado/headscale-gateway/internal/licensing"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommercialRestrictionPreservesAdministration(t *testing.T) {
	r := &gatewayRuntime{license: licensing.New(t.TempDir())}
	r.cfg.Store(&Config{AdminEnabled: true, AdminSubdomain: "admin", AdminDomainMode: "lan"})
	r.active.Store(&gatewaySnapshot{domain: "node.example.test", handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
	r.adminWeb.Store(&adminSiteHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
	for _, tc := range []struct {
		url  string
		code int
	}{{"https://service.node.example.test/", 402}, {"https://node.example.test/", 402}, {"https://service.node.example.test/pki/v1/public-ca", 402}, {"https://admin.node.example.test/admin/", 204}, {"https://node.example.test/pki/v1/public-ca", 204}} {
		req := httptest.NewRequest("GET", tc.url, nil)
		req.TLS = &tls.ConnectionState{}
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if out.Code != tc.code {
			t.Fatalf("%s: %d instead of %d", tc.url, out.Code, tc.code)
		}
	}
}
