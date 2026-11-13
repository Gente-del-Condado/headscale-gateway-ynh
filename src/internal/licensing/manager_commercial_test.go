//go:build commercial

package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCommercialManagerActivationAndExpiry(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	previous := PublicKey
	PublicKey = base64.RawURLEncoding.EncodeToString(pub)
	defer func() { PublicKey = previous }()
	dir := t.TempDir()
	m := New(dir)
	missing := m.Status()
	if missing.State != "missing" || missing.Allowed || missing.InstanceID == "" {
		t.Fatal(missing)
	}
	now := time.Now().Unix()
	claims := Claims{Version: 1, Product: Product, InstanceID: missing.InstanceID, LicenseID: "test", CustomerID: "test", NotBefore: now - 60, ExpiresAt: now + 60, GraceUntil: now + 120, Features: []string{"gateway"}}
	token, err := Sign(claims, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "subscription.license")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	m.checked = time.Time{}
	if s := m.Status(); !s.Allowed || s.State != "active" {
		t.Fatal(s)
	}
	claims.ExpiresAt = now - 10
	claims.GraceUntil = now - 1
	token, err = Sign(claims, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	m.checked = time.Time{}
	if s := m.Status(); s.Allowed || s.State != "expired" {
		t.Fatal(s)
	}
	PublicKey = ""
	m.checked = time.Time{}
	if s := m.Status(); s.Allowed || s.State != "configuration_error" {
		t.Fatal(s)
	}
}
