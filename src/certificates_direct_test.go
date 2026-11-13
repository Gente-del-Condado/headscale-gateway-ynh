package main

import (
	"crypto/tls"
	"testing"
)

func TestAutoUsesLocalForIPAndACMEForDNS(t *testing.T) {
	local := &tls.Certificate{}
	public := &tls.Certificate{}
	base := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return public, nil }}
	config := withLocalAddressCertificate(base, local)
	for _, name := range []string{"", "localhost", "192.168.1.20", "100.64.0.12", "fd7a:115c:a1e0::c"} {
		cert, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
		if err != nil || cert != local {
			t.Errorf("%s not local: %v", name, err)
		}
	}
	cert, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: "admin.gateway.example.test"})
	if err != nil || cert != public {
		t.Fatal("DNS lost ACME", err)
	}
	cert, _ = base.GetCertificate(&tls.ClientHelloInfo{})
	if cert != public {
		t.Fatal("mutated original config")
	}
}
