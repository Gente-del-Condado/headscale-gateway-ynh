package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
)

func TestAdminConsoleAccessAddress(t *testing.T) {
	for _, tc := range []struct {
		address string
		options admin.HTTPOptions
		want    string
	}{
		{"127.0.0.1:9091", admin.HTTPOptions{}, "http://127.0.0.1:9091/admin/"},
		{"[::1]:9091", admin.HTTPOptions{}, "http://[::1]:9091/admin/"},
		{"[::]:9091", admin.HTTPOptions{}, "http://127.0.0.1:9091/admin/"},
		{"127.0.0.1:9091", admin.HTTPOptions{BasePath: "/gateway"}, "http://127.0.0.1:9091/gateway/admin/"},
		{"[::]:62662", admin.HTTPOptions{BasePathHeader: "X-Ingress-Path", TrustedProxies: []string{"172.30.32.2"}}, ""},
		{"127.0.0.1:19091", admin.HTTPOptions{BasePath: "/gateway", TrustedProxies: []string{"127.0.0.1"}}, ""},
	} {
		if got := adminConsoleURL(tc.address, tc.options); got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.address, got, tc.want)
		}
	}
}

func TestAdminConsoleProxyHasNoMisleadingDirectLink(t *testing.T) {
	got := adminConsoleText("", "", false, false, false)
	if strings.Contains(got, "http://") || strings.Contains(got, "navegador de este equipo") || strings.Contains(got, "\x1b]8;;") {
		t.Fatal("proxy-only access advertised a local browser link")
	}
	if !strings.Contains(got, "proxy") {
		t.Fatal("missing proxy access guidance")
	}
}

func TestAdminConsoleCredentialVisibility(t *testing.T) {
	const code = "test-private-bootstrap-code"
	const url = "http://127.0.0.1:9091/admin/"
	for _, tc := range []struct {
		name               string
		interactive, color bool
	}{
		{"redirected", false, false}, {"plain terminal", true, false}, {"color terminal", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := adminConsoleText(url, code, true, tc.interactive, tc.color)
			if strings.Contains(got, code) != tc.interactive {
				t.Fatal("credential visibility does not match terminal status")
			}
			if !strings.Contains(got, url) {
				t.Fatal("missing local panel URL")
			}
			if strings.Contains(got, "\x1b") != tc.color {
				t.Fatal("incorrect ANSI behavior")
			}
			if tc.color && !strings.Contains(got, "\x1b]8;;"+url+"\x1b\\") {
				t.Fatal("hyperlink target must contain only panel URL")
			}
		})
	}
}

func TestAdminConsoleNoInventedCredential(t *testing.T) {
	got := adminConsoleText("http://[::1]:9091/admin/", "", false, true, false)
	if !strings.Contains(got, "--reset-recovery") || !strings.Contains(got, "--db") {
		t.Fatal("missing recovery instructions")
	}
	if strings.Contains(got, "15") {
		t.Fatal("must not announce a nonexistent code")
	}
}

func TestRecoveryRemainsPendingUntilConsoleDisplay(t *testing.T) {
	db, err := openCatalog(filepath.Join(t.TempDir(), "gateway.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, pending, err := getOrCreateRecoveryToken(db)
	if err != nil || !pending {
		t.Fatalf("first start pending=%v err=%v", pending, err)
	}
	again, pending, err := getOrCreateRecoveryToken(db)
	if err != nil || !pending || first != again {
		t.Fatal("interrupted startup lost pending recovery")
	}
	if err = db.PutJSON("recovery_bootstrap_pending_v1", false); err != nil {
		t.Fatal(err)
	}
	_, pending, err = getOrCreateRecoveryToken(db)
	if err != nil || pending {
		t.Fatal("displayed recovery should no longer be pending")
	}
}
