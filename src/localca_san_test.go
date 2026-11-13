package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestLocalCertificateIPCoverageAndRenewal(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), LocalCAValidity: 365 * 24 * time.Hour, LocalCertificateValidity: 24 * time.Hour, LocalCertificateIPs: []string{"192.168.1.8", "100.64.0.9", "fd7a:115c:a1e0::9", "0.0.0.0", "::"}}
	domain := "gateway.example.test"
	ca := newLocalCA(cfg.DataDir, domain)
	root, key, err := ensureLocalCA(context.Background(), cfg, ca)
	if err != nil {
		t.Fatal(err)
	}
	local := newLocalCertificate(cfg.DataDir, domain)
	_, cert, err := ensureLocalCertificate(context.Background(), cfg, root, key, domain, local)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{domain, "admin." + domain, "localhost", "127.0.0.1", "::1", "192.168.1.8", "100.64.0.9", "fd7a:115c:a1e0::9"} {
		if err := cert.VerifyHostname(host); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	if cert.VerifyHostname("0.0.0.0") == nil {
		t.Fatal("unspecified IP included")
	}
	_, same, err := ensureLocalCertificate(context.Background(), cfg, root, key, domain, local)
	if err != nil || same.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Fatal("unnecessary renewal", err)
	}
	cfg.LocalCertificateIPs = append(cfg.LocalCertificateIPs, "192.168.1.99")
	_, next, err := ensureLocalCertificate(context.Background(), cfg, root, key, domain, local)
	if err != nil {
		t.Fatal(err)
	}
	if next.SerialNumber.Cmp(cert.SerialNumber) == 0 || next.VerifyHostname("192.168.1.99") != nil {
		t.Fatal("missing renewal for new IP")
	}
	if err = next.CheckSignatureFrom(root); err != nil {
		t.Fatal("CA changed", err)
	}
	if _, err = os.Stat(ca.KeyPath); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCertificateRealTLSConnectionByIPAndPort(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), LocalCAValidity: 365 * 24 * time.Hour, LocalCertificateValidity: 24 * time.Hour}
	domain := "lab.example.test"
	ca, key, err := ensureLocalCA(context.Background(), cfg, newLocalCA(cfg.DataDir, domain))
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := ensureLocalCertificate(context.Background(), cfg, ca, key, domain, newLocalCertificate(cfg.DataDir, domain))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("TLS OK")) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "TLS OK" {
		t.Fatalf("unexpected response %q", body)
	}
}
