package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestManualVPNPausePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.sqlite")
	v, err := loadVPNState(path)
	if err != nil {
		t.Fatal(err)
	}
	if v.isPaused() {
		t.Fatal("paused by default")
	}
	a := &adminController{configPath: path, vpn: v}
	if err = a.saveVPNPause(true); err != nil {
		t.Fatal(err)
	}
	restored, err := loadVPNState(path)
	if err != nil || !restored.isPaused() {
		t.Fatalf("pause lost after reopen: %v", err)
	}
	if err = a.saveVPNPause(false); err != nil {
		t.Fatal(err)
	}
	restored, err = loadVPNState(path)
	if err != nil || restored.isPaused() {
		t.Fatalf("resume lost: %v", err)
	}
}

// La marca de transporte procede del listener, no de cabeceras del navegador.
// Una petición bloqueada debe dejar intactos memoria y estado persistido.
func TestPauseRejectsVPNIngressBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.sqlite")
	v, err := loadVPNState(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &adminController{configPath: path, vpn: v}
	ctx := context.WithValue(context.Background(), ingressContextKey{}, true)
	_, err = a.NetworkDisconnect(ctx)
	if err == nil || err.Error() != pauseFromVPNMessage {
		t.Fatalf("VPN pause not rejected: %v", err)
	}
	restored, err := loadVPNState(path)
	if err != nil || restored.isPaused() || v.isPaused() {
		t.Fatal("pause was changed", err)
	}
	for _, local := range []context.Context{context.Background(), context.WithValue(context.Background(), ingressContextKey{}, false)} {
		_, err = a.NetworkDisconnect(local)
		if err == nil || err.Error() == pauseFromVPNMessage {
			t.Fatalf("local access incorrectly blocked: %v", err)
		}
	}
}
func TestNetworkInfoReportsPauseProtection(t *testing.T) {
	runtime := &gatewayRuntime{}
	runtime.cfg.Store(&Config{NetworkPanelEnabled: false})
	a := &adminController{runtime: runtime}
	for _, tailnet := range []bool{true, false} {
		result, err := a.NetworkInfo(context.WithValue(context.Background(), ingressContextKey{}, tailnet))
		if err != nil {
			t.Fatal(err)
		}
		info := result.(map[string]any)
		if info["pause_allowed"] != !tailnet {
			t.Fatal(info)
		}
	}
}
