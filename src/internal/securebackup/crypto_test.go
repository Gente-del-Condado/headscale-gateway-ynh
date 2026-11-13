package securebackup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptArchiveAndRestore(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "key.json")
	if e := NewKey(keyPath, "una contraseña segura 2026"); e != nil {
		t.Fatal(e)
	}
	k, e := ReadKey(keyPath)
	if e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(root, "sql.db")
	os.WriteFile(src, []byte("private content"), 0600)
	archive := filepath.Join(root, "test.hgw")
	if _, e = Pack(context.Background(), archive, k, "sqlite", map[string]string{"data/gateway.sqlite": src}); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(archive)
	if bytes.Contains(raw, []byte("private content")) {
		t.Fatal("plaintext disclosure")
	}
	dest := filepath.Join(root, "restored")
	os.Mkdir(dest, 0700)
	if _, e = Extract(context.Background(), archive, "invalid password", dest); e == nil {
		t.Fatal("accepted wrong password")
	}
	if _, e = Extract(context.Background(), archive, "una contraseña segura 2026", dest); e != nil {
		t.Fatal(e)
	}
	file, _ := os.ReadFile(filepath.Join(dest, "data", "gateway.sqlite"))
	if string(file) != "private content" {
		t.Fatal("not restored")
	}
	raw[len(raw)-20] ^= 1
	os.WriteFile(archive, raw, 0600)
	dest2 := filepath.Join(root, "restored2")
	os.Mkdir(dest2, 0700)
	_, e = Extract(context.Background(), archive, "una contraseña segura 2026", dest2)
	if e == nil || e == io.EOF {
		t.Fatal("tampered archive accepted")
	}
}
