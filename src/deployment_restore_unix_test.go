//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func TestRestoreLeavesRejectedDirectoriesIntact(t *testing.T) {
	parent := t.TempDir()
	active := filepath.Join(parent, "gateway", "gateway.sqlite")
	for _, tc := range []struct {
		name   string
		marker string
		mode   os.FileMode
	}{
		{"unrelated", active, 0700},
		{".hgw-restore-unmarked", "", 0700},
		{".hgw-restore-other", filepath.Join(parent, "another", "gateway.sqlite"), 0700},
		{".hgw-restore-insecure", active, 0755},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(parent, tc.name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tc.mode); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "keep")
			if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.marker != "" {
				if err := os.WriteFile(filepath.Join(dir, ".validated"), []byte(tc.marker), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := applyStagedRestore(active, dir); err == nil {
				t.Fatal("se aceptó una carpeta ajena")
			}
			if contents, err := os.ReadFile(file); err != nil || string(contents) != "original" {
				t.Fatal("la restauración modificó la carpeta rechazada", err)
			}
		})
	}
}

func TestFullRestorePreservesPlatformParentAndOptions(t *testing.T) {
	parent := t.TempDir()
	options := filepath.Join(parent, "options.json")
	if err := os.WriteFile(options, []byte("platform-options"), 0600); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(parent, "gateway", "gateway.sqlite")
	db, err := state.Open(active)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PutJSON("restore-probe", "snapshot"); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(parent, ".hgw-restore-")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BackupTo(filepath.Join(stage, "data")); err != nil {
		t.Fatal(err)
	}
	if err := db.PutJSON("restore-probe", "newer"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{".validated": active, ".mode": "all"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := applyStagedRestore(active, stage); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(options); err != nil || string(got) != "platform-options" {
		t.Fatal("platform parent was changed", err)
	}
	db, err = state.Open(active)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if found, err := db.GetJSON("restore-probe", &value); err != nil || !found || value != "snapshot" {
		t.Fatal("restored state", value, err)
	}
}
