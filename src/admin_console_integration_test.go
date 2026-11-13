package main

import (
	"os"
	"testing"
)

// Solo se invoca en un proceso hijo desechable desde tools/check_console.py.
func TestConsoleIntegrationProcess(t *testing.T) {
	path := os.Getenv("HGW_CONSOLE_FIXTURE_DB")
	if path == "" {
		t.Skip("isolated console integration helper")
	}
	if err := ensureSetupDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := openCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	settings := initialSettings()
	settings["admin.listen_addr"] = "127.0.0.1:0"
	if err := db.ReplaceSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	startGateway(path)
}
