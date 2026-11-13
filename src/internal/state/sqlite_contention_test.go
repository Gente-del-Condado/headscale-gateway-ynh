//go:build !legacy_sqlite

package state

import (
	"path/filepath"
	"testing"
)

func TestOpenCurrentCatalogWhileWriterIsActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.sqlite")
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err = writer.InitCatalog(); err != nil {
		t.Fatal(err)
	}
	if err = writer.PutJSON("acme.cooldown.test", map[string]string{"until": "future"}); err != nil {
		t.Fatal(err)
	}
	tx, err := writer.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE metadata SET value=value WHERE key=?", "acme.cooldown.test"); err != nil {
		t.Fatal(err)
	}
	// Mantiene abierto el escritor. Los lectores WAL no deben necesitar su bloqueo
	// para abrir un catálogo existente o leer metadatos cifrados.
	reader, err := Open(path)
	if err != nil {
		t.Fatalf("Open under writer: %v", err)
	}
	defer reader.Close()
	if err = reader.InitCatalog(); err != nil {
		t.Fatalf("current InitCatalog attempted a write: %v", err)
	}
	var value map[string]string
	if found, err := reader.GetJSON("acme.cooldown.test", &value); err != nil || !found || value["until"] != "future" {
		t.Fatalf("read under writer: %v %v %v", found, value, err)
	}
}
