package configtransfer

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoundTripAndTamper(t *testing.T) {
	values := map[string]string{"headscale.url": "https://vpn.example.test", "headscale.api_key": "secret-test"}
	encrypted, err := Export(values, true, "una contraseña muy larga de prueba")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("secret-test")) {
		t.Fatal("credencial expuesta")
	}
	profile, err := Import(encrypted, "una contraseña muy larga de prueba")
	if err != nil || profile.Settings["headscale.api_key"] != "secret-test" {
		t.Fatalf("importación: %v", err)
	}
	if _, err = Import(encrypted, "contraseña incorrecta"); err == nil {
		t.Fatal("aceptó contraseña falsa")
	}
	tampered := append([]byte(nil), encrypted...)
	tampered[len(tampered)-1] ^= 0xFF
	if _, err = Import(tampered, "una contraseña muy larga de prueba"); err == nil {
		t.Fatal("aceptó archivo manipulado")
	}
	if _, err = Export(values, true, "corta"); err == nil {
		t.Fatal("contraseña corta admitida")
	}
	if _, err = Import([]byte(strings.Repeat("x", maxCipher+1)), "x"); err == nil {
		t.Fatal("archivo enorme admitido")
	}
}
