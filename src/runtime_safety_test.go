package main

import (
	"crypto/tls"
	"strings"
	"testing"
)

func TestRuntimeSinSnapshotNoProducePanic(t *testing.T) {
	r := &gatewayRuntime{}
	if r.domain() != "" {
		t.Fatal("dominio no inicializado")
	}
	if cfg, err := r.GetConfigForClient(nil); err == nil || cfg != nil {
		t.Fatalf("TLS debería informar de falta de certificado, cfg=%v err=%v", cfg, err)
	}
}

func TestRuntimeCertificadoPublicado(t *testing.T) {
	r := &gatewayRuntime{}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	r.active.Store(&gatewaySnapshot{domain: "test.example", tlsConfig: cfg})
	got, err := r.GetConfigForClient(nil)
	if err != nil || got != cfg || !strings.EqualFold(r.domain(), "test.example") {
		t.Fatalf("snapshot inesperado: %v %v", got, err)
	}
}
