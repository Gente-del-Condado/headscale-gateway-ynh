package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublishedAdminAccess(t *testing.T) {
	for _, tc := range []struct {
		name, mode, host string
		tailnet          bool
		want             int
	}{
		{"default disabled", "off", "admin.pc.intra.example", true, 404},
		{"tailnet allowed", "tailnet", "admin.pc.intra.example", true, 204},
		{"LAN blocked", "tailnet", "admin.pc.intra.example", false, 404},
		{"LAN explicitly allowed", "all", "admin.pc.intra.example", false, 204},
		{"other host blocked", "all", "other.pc.intra.example", true, 404},
		{"port in Host", "tailnet", "admin.pc.intra.example:443", true, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{AdminEnabled: true, AdminDomainMode: tc.mode, AdminSubdomain: "admin"}
			runtime := &gatewayRuntime{}
			runtime.cfg.Store(&cfg)
			runtime.active.Store(&gatewaySnapshot{domain: "pc.intra.example", handler: http.NotFoundHandler()})
			runtime.adminWeb.Store(&adminSiteHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
			request := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/admin/", nil)
			request.Header.Set("X-Forwarded-For", "100.100.100.100")
			request.Header.Set("X-Forwarded-Proto", "https")
			request.Header.Set("X-Forwarded-Host", "admin.pc.intra.example")
			result := httptest.NewRecorder()
			withGatewayIngress(runtime, tc.tailnet).ServeHTTP(result, request)
			if result.Code != tc.want {
				t.Fatalf("status %d, esperado %d", result.Code, tc.want)
			}
		})
	}
}

func TestPublishedAdminRequiresHTTPS(t *testing.T) {
	cfg := Config{AdminEnabled: true, AdminDomainMode: "all", AdminSubdomain: "admin"}
	runtime := &gatewayRuntime{}
	runtime.cfg.Store(&cfg)
	runtime.active.Store(&gatewaySnapshot{domain: "pc.intra.example", handler: http.NotFoundHandler()})
	runtime.adminWeb.Store(&adminSiteHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
	result := httptest.NewRecorder()
	withGatewayIngress(runtime, true).ServeHTTP(result, httptest.NewRequest("GET", "http://admin.pc.intra.example/admin/", nil))
	if result.Code != 404 {
		t.Fatalf("HTTP sin TLS debe rechazarse: %d", result.Code)
	}
}

func TestPublishedAdminByCertificateIPPreservesScope(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), AdminEnabled: true, AdminDomainMode: "all", AdminSubdomain: "admin", LocalCAValidity: 24 * time.Hour, LocalCertificateValidity: time.Hour, LocalCertificateIPs: []string{"192.168.1.20", "fd7a:115c:a1e0::c"}}
	domain := "gateway.example.test"
	ca, key, err := ensureLocalCA(context.Background(), cfg, newLocalCA(cfg.DataDir, domain))
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := ensureLocalCertificate(context.Background(), cfg, ca, key, domain, newLocalCertificate(cfg.DataDir, domain))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &gatewayRuntime{}
	runtime.cfg.Store(&cfg)
	runtime.active.Store(&gatewaySnapshot{domain: domain, handler: http.NotFoundHandler(), certificates: &certificateManager{localTLSCertificate: cert}})
	runtime.adminWeb.Store(&adminSiteHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })})
	for _, tc := range []struct {
		mode, host string
		tailnet    bool
		want       int
	}{
		{"all", "192.168.1.20:8443", false, 204}, {"all", "[fd7a:115c:a1e0::c]", true, 204}, {"all", "[fd7a:115c:a1e0::c]:8443", true, 204}, {"tailnet", "192.168.1.20:8443", false, 404}, {"tailnet", "192.168.1.20:8443", true, 204}, {"off", "192.168.1.20:8443", true, 404}, {"all", "192.168.1.99:8443", false, 404},
	} {
		next := cfg
		next.AdminDomainMode = tc.mode
		runtime.cfg.Store(&next)
		w := httptest.NewRecorder()
		withGatewayIngress(runtime, tc.tailnet).ServeHTTP(w, httptest.NewRequest("GET", "https://"+tc.host+"/admin/", nil))
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
}
