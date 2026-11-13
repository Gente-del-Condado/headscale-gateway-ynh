package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPublicCALANIsolation(t *testing.T) {
	calls := 0
	handler := publicCALANHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials forwarded")
		}
		w.WriteHeader(200)
	}))
	for _, tc := range []struct {
		method, path, remote string
		want                 int
	}{{"GET", "/pki/v1/public-ca", "192.168.1.20:5000", 200}, {"HEAD", "/pki/v1/public-ca", "192.168.1.20:5000", 200}, {"GET", "/admin/", "192.168.1.20:5000", 404}, {"GET", "/api/v1/status", "192.168.1.20:5000", 404}, {"POST", "/pki/v1/public-ca", "192.168.1.20:5000", 405}, {"GET", "/pki/v1/public-ca?token=x", "192.168.1.20:5000", 404}, {"GET", "/pki/v1/public-ca", "8.8.8.8:5000", 403}, {"GET", "/pki/v1/public-ca", "100.64.0.2:5000", 403}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("Authorization", "Bearer test")
		r.Header.Set("Cookie", "test=value")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
	if calls != 2 {
		t.Fatal("non-public request reached handler", calls)
	}
}

func TestPublicCAPortCollisionDoesNotStopAdministration(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	runtime := &gatewayRuntime{}
	startPublicCALAN(context.Background(), Config{CAHTTPEnabled: true, CAHTTPListenAddr: listener.Addr().String()}, runtime, http.NotFoundHandler())
	if runtime.caHTTPPort.Load() != 0 {
		t.Fatal("occupied listener advertised")
	}
}

func TestCommunicationSettingsValidation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gateway.sqlite")
	if err := ensureSetupDatabase(dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := openCatalog(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	values := initialSettings()
	values["server.listen_addr"] = "0.0.0.0:19443"
	values["pki.http_enabled"] = "true"
	values["pki.http_listen_addr"] = "0.0.0.0:19080"
	values["pki.discovery_port"] = "47992"
	cfg, err := configFromSettings(dbPath, values, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStagedConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.CAHTTPListenAddr = "0.0.0.0:19443"
	if err := validateStagedConfig(cfg); err == nil {
		t.Fatal("HTTPS port collision accepted")
	}
	values["pki.discovery_port"] = "65536"
	if _, err := configFromSettings(dbPath, values, db); err == nil {
		t.Fatal("invalid UDP port accepted")
	}
}
