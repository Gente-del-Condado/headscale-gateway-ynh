package pki

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveLocalIndependentOfSystemPrivileges(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "local-ca", "test.example")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	pem := rootForTest(t, true)
	root, err := ParseRoot([]byte(pem))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "ca.crt"), []byte(pem), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "ca.key"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if RemoveLocal(dir, "test.example", "wrong") == nil {
		t.Fatal("accepted mismatched fingerprint")
	}
	if RemoveLocal(dir, "../test.example", root.Fingerprint) == nil {
		t.Fatal("accepted traversal")
	}
	if err = RemoveLocal(dir, "test.example", root.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("local CA still present", err)
	}
	public, err := os.ReadFile(filepath.Join(dir, "pki", "removed-public-ca", root.Fingerprint+".crt"))
	if err != nil || string(public) != pem {
		t.Fatal("missing public recovery certificate", err)
	}
}
