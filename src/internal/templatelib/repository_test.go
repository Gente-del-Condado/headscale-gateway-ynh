package templatelib

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestJSONBundle(t *testing.T) {
	raw := []byte(`{"format":"headscale-gateway-templates","version":1,"templates":[{"name":"prueba"}]}`)
	b, err := Decode[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil || len(b.Templates) != 1 || b.Templates[0].Name != "prueba" {
		t.Fatalf("importación: %+v %v", b, err)
	}
	for _, test := range []string{
		`{"format":"incorrect","version":1,"templates":[{}]}`,
		`{"format":"headscale-gateway-templates","version":2,"templates":[{}]}`,
		`{"format":"headscale-gateway-templates","version":1,"templates":[]}`,
	} {
		if _, err := Decode[map[string]any]([]byte(test)); err == nil {
			t.Fatal("validó datos no admitidos", test)
		}
	}
}
func TestPublicIPGuards(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "100.100.100.100", "169.254.169.254", "198.18.1.1", "::1", "fc00::1", "2001:db8::1"} {
		if safeIP(net.ParseIP(host)) {
			t.Fatalf("IP privada/reservada admitida: %s", host)
		}
	}
	if !safeIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("IP pública rechazada")
	}
	if permittedIP(net.ParseIP("169.254.169.254"), true) {
		t.Fatal("metadata permitida con opción privada")
	}
	if !permittedIP(net.ParseIP("100.100.100.100"), true) {
		t.Fatal("CGNAT privada bloqueada expresamente")
	}
}
func TestInvalidRepositoryURL(t *testing.T) {
	for _, host := range []string{"http://example.org/templates.json", "https://user:pass@example.org/templates.json", "https://example.org:99999/templates.json", "https://example.org/path#fragment"} {
		_, err := Fetch(context.Background(), host, false)
		if err == nil {
			t.Fatal("URL inválida admitida", host)
		}
	}
	_, err := Fetch(context.Background(), strings.Repeat("x", 2049), false)
	if err == nil {
		t.Fatal("URL muy larga admitida")
	}
}
