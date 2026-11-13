package portal

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFederationTLSBoundsAndPinnedTailnetDial(t *testing.T) {
	mode := "ok"
	origin := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != CatalogPath {
			t.Error("queried a non-catalog path", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials forwarded")
		}
		switch mode {
		case "redirect":
			http.Redirect(w, r, "https://other.test", 302)
			return
		case "large":
			_, _ = w.Write([]byte(strings.Repeat("x", 1024*1024+1)))
			return
		}
		c := Catalog{Protocol: "headscale-gateway", Version: 1, Gateway: "Lab", Origin: origin, Services: []Entry{{ID: "app", Name: "App", URL: "https://app." + strings.TrimPrefix(origin, "https://")}}}
		if mode == "external" {
			c.Services[0].URL = "https://outside.test"
		}
		_ = json.NewEncoder(w).Encode(c)
	}))
	defer server.Close()
	certificate := server.Certificate()
	if len(certificate.DNSNames) == 0 {
		t.Fatal("fixture has no DNS name")
	}
	origin = "https://" + certificate.DNSNames[0]
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
	if _, err := x509.ParseCertificate(certificate.Raw); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		calls++
		if address != "100.64.1.2:443" {
			t.Error("not pinned", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	resolve := func(context.Context, string) (string, error) { return "100.64.1.2", nil }
	c, err := fetchResolved(context.Background(), origin, ca, dial, resolve)
	if err != nil || len(c.Services) != 1 || calls != 1 {
		t.Fatal(c, err, calls)
	}
	if _, err := fetchResolved(context.Background(), origin+":8443", ca, dial, resolve); err != nil {
		t.Fatal("legacy Tailnet port not corrected", err)
	}
	for _, m := range []string{"redirect", "large", "external"} {
		mode = m
		if _, err := fetchResolved(context.Background(), origin, ca, dial, resolve); err == nil {
			t.Fatal("invalid response accepted", m)
		}
	}
	mode = "ok"
	if _, err := fetchResolved(context.Background(), origin, "", dial, resolve); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	if _, err := fetchResolved(context.Background(), origin, ca, nil, resolve); err == nil {
		t.Fatal("tailnet fell back to OS")
	}
	if _, err := Fetch(context.Background(), "https://127.0.0.1", ca, dial); err == nil {
		t.Fatal("loopback destination accepted")
	}
}
