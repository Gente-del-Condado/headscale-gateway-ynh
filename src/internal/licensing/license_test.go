package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignedSubscription(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	c := Claims{Version: 1, Product: Product, LicenseID: "test-license", CustomerID: "test-customer", InstanceID: "test-instance", NotBefore: 100, ExpiresAt: 200, GraceUntil: 300, Features: []string{"gateway"}}
	token, err := Sign(c, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		when  int64
		state string
		ok    bool
	}{{99, "not_yet_valid", false}, {100, "active", true}, {199, "active", true}, {200, "grace", true}, {299, "grace", true}, {300, "expired", false}} {
		_, state, e := Verify(token, pub, c.InstanceID, time.Unix(tc.when, 0))
		if state != tc.state || (e == nil) != tc.ok {
			t.Fatalf("time %d: %s %v", tc.when, state, e)
		}
	}
	if _, _, e := Verify(token, pub, "another-instance", time.Unix(150, 0)); e == nil {
		t.Fatal("clone identity accepted")
	}
	if _, _, e := Verify("x"+token, pub, c.InstanceID, time.Unix(150, 0)); e == nil {
		t.Fatal("tampering accepted")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, _, e := Verify(token, other, c.InstanceID, time.Unix(150, 0)); e == nil {
		t.Fatal("wrong issuer accepted")
	}
	c.Features = nil
	if _, e := Sign(c, key); e == nil {
		t.Fatal("missing feature accepted")
	}
	c.Features = []string{"gateway"}
	c.GraceUntil = c.ExpiresAt + 7*24*3600 + 1
	if _, e := Sign(c, key); e == nil {
		t.Fatal("excessive grace accepted")
	}
	if _, _, e := Verify(strings.Repeat("x", MaxSize+1), pub, c.InstanceID, time.Unix(150, 0)); e == nil {
		t.Fatal("oversize accepted")
	}
}
func TestInstancePersists(t *testing.T) {
	dir := t.TempDir()
	a, e := Instance(dir)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Instance(dir)
	if e != nil || a != b || len(a) != 64 {
		t.Fatal("identity not stable")
	}
	if e := os.WriteFile(filepath.Join(dir, "license-instance.id"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Instance(dir); e == nil {
		t.Fatal("corrupt identity replaced silently")
	}
}
func TestDefaultBuildDoesNotTouchDisk(t *testing.T) {
	if Enabled {
		t.Skip("commercial build")
	}
	dir := filepath.Join(t.TempDir(), "absent")
	if s := New(dir).Status(); !s.Allowed || s.Enabled || s.State != "disabled" {
		t.Fatal(s)
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("disabled module wrote data")
	}
}
