package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func portalFixture(t *testing.T) (*gatewayRuntime, *adminController) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gateway.sqlite")
	db, err := openCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	cfg := Config{DatabasePath: path, DataDir: filepath.Dir(path), Services: map[string]string{"visible": "http://backend-secret", "hidden": "http://secret-other"}, Portal: portal.Default()}
	cfg.Portal.Local["visible"] = portal.Local{Share: true}
	cfg.Portal.Local["hidden"] = portal.Local{Hidden: true, Share: true}
	r := &gatewayRuntime{}
	r.cfg.Store(&cfg)
	r.active.Store(&gatewaySnapshot{domain: "pc.zone.test", handler: http.NotFoundHandler()})
	return r, &adminController{runtime: r, configPath: path}
}
func TestPortalCatalogIsOptInAndRedacted(t *testing.T) {
	r, _ := portalFixture(t)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "https://pc.zone.test"+path, nil))
		return w
	}
	if got := get(portal.CatalogPath); got.Code != 404 {
		t.Fatal(got.Code)
	}
	cfg := r.currentConfig()
	cfg.Portal.Publish = true
	r.cfg.Store(&cfg)
	response := get(portal.CatalogPath)
	if response.Code != 200 {
		t.Fatal(response.Code)
	}
	var catalog portal.Catalog
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Services) != 1 || catalog.Services[0].ID != "visible" || strings.Contains(response.Body.String(), "backend-secret") {
		t.Fatal(catalog)
	}
	cfg.Portal.Enabled = false
	r.cfg.Store(&cfg)
	if get("/").Code != 404 || get(portal.CatalogPath).Code != 200 {
		t.Fatal("modules coupled")
	}
}
func TestPortalSavePersistsWithoutRestartAndRejectsUnknownPeer(t *testing.T) {
	r, a := portalFixture(t)
	cfg := r.currentConfig().Portal
	cfg.Title = "My private portal"
	if _, err := a.PortalSave(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	db, err := openCatalog(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var saved portal.Config
	if _, err = db.GetJSON(portal.SettingsKey, &saved); err != nil || saved.Title != cfg.Title {
		t.Fatal(saved, err)
	}
	if r.currentConfig().Portal.Title != cfg.Title {
		t.Fatal("runtime not updated")
	}
	cfg.Selected = []portal.Selection{{Peer: strings.Repeat("a", 64), ID: "app"}}
	if _, err = a.PortalSave(context.Background(), cfg); err == nil {
		t.Fatal("unknown peer accepted")
	}
}

func TestRemoteEntriesExpireAndRespectRevocation(t *testing.T) {
	r, _ := portalFixture(t)
	cfg := r.currentConfig()
	cfg.Portal.Federation = true
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Lab CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := pki.ParseRoot(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
	if err != nil {
		t.Fatal(err)
	}
	anchor.Name = "Lab"
	anchor.Endpoint = "https://other.zone.test"
	anchor.Approved = true
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Put(anchor); err != nil {
		t.Fatal(err)
	}
	cfg.Portal.Selected = []portal.Selection{{Peer: anchor.Fingerprint, ID: "app"}}
	cache := portalPeerStatus{Peer: anchor.Fingerprint, Endpoint: anchor.Endpoint, Checked: time.Now(), Catalog: portal.Catalog{Services: []portal.Entry{{ID: "app", URL: "https://app.other.zone.test"}}}}
	r.portalCache = map[string]portalPeerStatus{anchor.Fingerprint: cache}
	if len(r.remotePortalEntries(cfg)) != 1 {
		t.Fatal("selected service absent")
	}
	cache.Checked = time.Now().Add(-4 * time.Minute)
	r.portalCache[anchor.Fingerprint] = cache
	if len(r.remotePortalEntries(cfg)) != 0 {
		t.Fatal("expired service still visible")
	}
	cache.Checked = time.Now()
	r.portalCache[anchor.Fingerprint] = cache
	if err = store.Delete(anchor.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if len(r.remotePortalEntries(cfg)) != 0 {
		t.Fatal("revoked service still visible")
	}
}

func TestPortalProbeRequiresEnabledFederationAndApprovedPeer(t *testing.T) {
	r, a := portalFixture(t)
	if _, err := a.PortalProbe(context.Background(), strings.Repeat("a", 64)); err == nil {
		t.Fatal("disabled federation accepted")
	}
	cfg := r.currentConfig()
	cfg.Portal.Federation = true
	r.cfg.Store(&cfg)
	for _, id := range []string{strings.Repeat("a", 64), "../../gateway.sqlite", "https://127.0.0.1"} {
		if _, err := a.PortalProbe(context.Background(), id); err == nil {
			t.Fatal("unapproved peer accepted", id)
		}
	}
}
