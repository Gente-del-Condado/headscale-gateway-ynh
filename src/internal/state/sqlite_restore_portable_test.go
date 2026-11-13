package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRestoreOffline(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gateway.sqlite")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.PutJSON("example.setting", map[string]string{"secret": "before"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backups", "manual")
	if err = os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
		t.Fatal(err)
	}
	if err = db.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	if err = db.PutJSON("example.setting", map[string]string{"secret": "after"}); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	previous, err := RestoreOffline(dbPath, backup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(previous); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var v map[string]string
	ok, err := reopened.GetJSON("example.setting", &v)
	if err != nil || !ok || v["secret"] != "before" {
		t.Fatalf("restore incorrecta: %+v %t %v", v, ok, err)
	}
}
func TestRestoreRejectsBadBackup(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "bad")
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "gateway.sqlite"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "master.key"), make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreOffline(filepath.Join(dir, "live.sqlite"), backup); err == nil {
		t.Fatal("copia malformada aceptada")
	}
}
