package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "gateway.log")
	w := &rotatingWriter{path: path, maxBytes: 15, backups: 2}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := w.reopen(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.file.Close() }()
	for _, str := range []string{"0000000000", "1111111111", "2222222222", "3333333333"} {
		if _, err := w.Write([]byte(str)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"", ".1", ".2"} {
		if _, err := os.Stat(path + s); err != nil {
			t.Fatalf("falta %s: %v", s, err)
		}
	}
	first, _ := os.ReadFile(path + ".2")
	if !bytes.Equal(first, []byte("1111111111")) {
		t.Fatalf("rotación incorrecta: %q", first)
	}
}

func TestTruncateRefusesDifferentFile(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "active.log")
	replacement := filepath.Join(dir, "other.log")
	w := &rotatingWriter{path: original, maxBytes: 1024, backups: 1}
	if err := w.reopen(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.file.Close() }()
	if _, err := w.Write([]byte("active")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	w.path = replacement
	if err := w.truncate(); err == nil {
		t.Fatal("truncated a different file")
	}
	for name, want := range map[string]string{original: "active", replacement: "keep"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("file changed: %s: %q %v", name, got, err)
		}
	}
}
