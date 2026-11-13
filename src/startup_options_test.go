package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func TestStartupBootstrapNeverResetsExistingCredentials(t *testing.T) {
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "gateway.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := admin.NewAuthManagerSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "initial.json")
	write := func(password string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(`{"username":"admin","password":"`+password+`"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	d := &startupOptions{InitialAdminFile: p}
	write("initial-password-123")
	if err := initializeAdministrator(db, a, d); err != nil {
		t.Fatal(err)
	}
	if !a.VerifyLocal("admin", "initial-password-123") {
		t.Fatal("initial login")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("bootstrap secret was retained")
	}
	if err := a.Update(admin.AuthUpdate{Mode: "local", LocalUsername: "admin", LocalPassword: "changed-password-123"}); err != nil {
		t.Fatal(err)
	}
	write("initial-password-123")
	if err := initializeAdministrator(db, a, d); err != nil {
		t.Fatal(err)
	}
	if !a.VerifyLocal("admin", "changed-password-123") || a.VerifyLocal("admin", "initial-password-123") {
		t.Fatal("upgrade reset password")
	}
}

func TestDefaultStartupSettings(t *testing.T) {
	t.Setenv("GATEWAY_BOOTSTRAP_FILE", "")
	settings := initialSettings()
	if err := startupInitialSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings["admin.listen_addr"] != "127.0.0.1:9091" || settings["server.listen_addr"] != "0.0.0.0:8443" {
		t.Fatal(settings)
	}
}

func TestStartupAdoptsAndPinsExistingVPNIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.sqlite")
	if err := os.Mkdir(filepath.Join(dir, "tailscale-state-original"), 0700); err != nil {
		t.Fatal(err)
	}
	d := &startupOptions{NodeName: "container"}
	name, err := stateHostname(path, d, "random-hostname")
	if err != nil || name != "original" {
		t.Fatal(name, err)
	}
	d.NodeName = "changed-option"
	name, err = stateHostname(path, nil, "another-container")
	if err != nil || name != "original" {
		t.Fatal("identity changed", name, err)
	}
}
