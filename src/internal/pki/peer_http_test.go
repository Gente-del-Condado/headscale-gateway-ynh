package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNetworkPlanPrefersTailnetCorrectPortAndPinsLAN(t *testing.T) {
	u, _ := url.Parse("https://node.test:8443")
	targets, err := planIPs(u, []string{"192.168.1.71", "100.64.0.13", "127.0.0.1", "8.8.8.8", "100.64.0.13"})
	if err != nil || len(targets) != 2 || targets[0].Origin != "https://node.test" || targets[0].IP != "100.64.0.13" || targets[1].Origin != "https://node.test:8443" {
		t.Fatal(targets, err)
	}
	u, _ = url.Parse("https://node.test")
	targets, _ = planIPs(u, []string{"192.168.1.71"})
	if targets[0].Origin != "https://node.test:8443" {
		t.Fatal(targets)
	}
}
func TestCAOnlyBootstrapAndNoAutomaticTrust(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := &x509.Certificate{SerialNumber: big.NewInt(19), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"gateway.test", "admin.gateway.test"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	pk, _ := x509.MarshalECPrivateKey(key)
	pair, _ := tls.X509KeyPair(ca, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: pk}))
	mode := "ok"
	requests := 0
	host := ""
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		host = r.Host
		if r.URL.Path != "/pki/v1/public-ca" {
			t.Error("wrong resource")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials leaked")
		}
		switch mode {
		case "redirect":
			http.Redirect(w, r, "https://elsewhere.test", 302)
		case "large":
			w.Write([]byte(strings.Repeat("x", maxPEMSize+1)))
		case "invalid":
			w.Write([]byte("not a CA"))
		default:
			w.Write(ca)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	defer server.Close()
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != "100.64.0.13:443" {
			t.Error(addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	target := Target{Origin: "https://gateway.test", IP: "100.64.0.13", Network: "Tailnet"}
	anchor, err := DownloadCA(context.Background(), []Target{target}, dial)
	if err != nil || anchor.Approved || anchor.Endpoint != target.Origin || host != "admin.gateway.test" || requests != 1 {
		t.Fatal(anchor, err, host, requests)
	}
	store, _ := NewStore(t.TempDir())
	anchor.Name = "fixture"
	anchor.Approved = true
	saved, err := store.ImportPending(anchor)
	if err != nil || saved.Approved {
		t.Fatal("bootstrap approved a CA", err)
	}
	saved.Approved = true
	saved.Name = "preserve"
	if err = store.Put(saved); err != nil {
		t.Fatal(err)
	}
	next, err := store.ImportPending(anchor)
	if err != nil || !next.Approved || next.Name != "preserve" {
		t.Fatal("existing trust overwritten", next, err)
	}
	// Normal federation still fails for unknown roots, even after a pending fetch.
	if _, err = GetPublic(context.Background(), target, "/pki/v1/public-ca", "", false, dial, maxPEMSize); err == nil {
		t.Fatal("unapproved TLS accepted")
	}
	if _, err = GetPublic(context.Background(), target, "/.well-known/headscale-gateway", "", true, dial, maxPEMSize); err == nil {
		t.Fatal("bootstrap applied to catalog")
	}
	if _, err = GetPublic(context.Background(), target, "/pki/v1/public-ca", string(ca), false, nil, maxPEMSize); err == nil {
		t.Fatal("used OS network for tailnet")
	}
	if _, err = GetPublic(context.Background(), target, "/api/v1/config", "", true, dial, maxPEMSize); err == nil {
		t.Fatal("arbitrary resource fetched")
	}
	for _, m := range []string{"redirect", "large", "invalid"} {
		mode = m
		if _, err = DownloadCA(context.Background(), []Target{target}, dial); err == nil {
			t.Fatal("unsafe response accepted", m)
		}
	}
	mode = "ok"
	target.Origin = "https://wrong.test"
	if _, err = DownloadCA(context.Background(), []Target{target}, dial); err == nil {
		t.Fatal("hostname mismatch accepted")
	}
}

func TestCADownloadFallsBackToLANWithoutApproval(t *testing.T) {
	ca := rootForTest(t, true)
	order := []string{}
	targets := []Target{{Origin: "https://node.test", IP: "100.64.0.13", Network: "Tailnet"}, {Origin: "https://node.test:8443", IP: "192.168.1.71", Network: "LAN"}}
	get := func(ctx context.Context, tg Target, path, roots string, pending bool, dial PeerDial, limit int64) ([]byte, error) {
		order = append(order, tg.Network+" "+tg.Origin)
		if !pending || roots != "" || path != "/pki/v1/public-ca" {
			t.Fatal("unsafe bootstrap")
		}
		if tg.Network == "Tailnet" {
			return nil, context.DeadlineExceeded
		}
		return []byte(ca), nil
	}
	a, err := downloadCAWith(context.Background(), targets, nil, get)
	if err != nil || a.Approved || a.Endpoint != "https://node.test:8443" || len(order) != 3 || order[0] != "Tailnet https://admin.node.test" || order[1] != "Tailnet https://node.test" || order[2] != "LAN https://admin.node.test:8443" {
		t.Fatal(a, err, order)
	}
}

func TestCustomHTTPSPortsAndHTTPBootstrapBoundaries(t *testing.T) {
	for _, origin := range []string{"https://node.test:9443", "https://node.test:10443"} {
		u, err := PeerURL(origin)
		if err != nil {
			t.Fatal(err)
		}
		targets, err := planIPs(u, []string{"192.168.1.10", "100.64.0.10"})
		if err != nil {
			t.Fatal(err)
		}
		if targets[0].Origin != "https://node.test" || targets[1].Origin != origin {
			t.Fatal(targets)
		}
	}
	target := Target{Origin: "http://node.test:19080", IP: "192.168.1.10", Network: "LAN"}
	for _, tc := range []struct {
		path      string
		bootstrap bool
	}{{"/.well-known/headscale-gateway", true}, {"/pki/v1/public-ca", false}, {"/api/v1/status", true}} {
		if _, err := GetPublic(context.Background(), target, tc.path, "", tc.bootstrap, nil, 64<<10); err == nil {
			t.Fatal("unsafe HTTP request accepted", tc)
		}
	}
	target.IP = "100.64.0.10"
	if _, err := GetPublic(context.Background(), target, "/pki/v1/public-ca", "", true, nil, 64<<10); err == nil {
		t.Fatal("HTTP tailnet accepted")
	}
	target.IP = "8.8.8.8"
	if _, err := GetPublic(context.Background(), target, "/pki/v1/public-ca", "", true, nil, 64<<10); err == nil {
		t.Fatal("public HTTP target accepted")
	}
}

func TestHTTPDownloadIsPinnedAndUntrusted(t *testing.T) {
	ca := rootForTest(t, true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pki/v1/public-ca" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected request")
		}
		if r.TLS != nil {
			t.Error("expected plaintext bootstrap")
		}
		w.Write([]byte(ca))
	}))
	defer server.Close()
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "192.168.1.20:19080" {
			t.Error("address not pinned", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	get := func(ctx context.Context, target Target, path, ca string, bootstrap bool, _ PeerDial, limit int64) ([]byte, error) {
		return getPublicWithSystemDial(ctx, target, path, ca, bootstrap, nil, limit, dial)
	}
	anchor, err := downloadCAWith(context.Background(), []Target{{Origin: "http://node.test:19080", IP: "192.168.1.20", Network: "LAN"}}, nil, get)
	if err != nil || anchor.Approved || anchor.PEM != ca {
		t.Fatal("HTTP CA should remain pending", err)
	}
}
