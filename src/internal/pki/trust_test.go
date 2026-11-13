package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/gentedelcondado/headscale-gateway/internal/platformfs"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

func rootForTest(t *testing.T, isCA bool) string {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "Gateway prueba"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), IsCA: isCA, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	raw, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
}
func TestParseRootRejectsDangerousInput(t *testing.T) {
	ca := rootForTest(t, true)
	item, e := ParseRoot([]byte(ca))
	if e != nil {
		t.Fatal(e)
	}
	if len(item.Fingerprint) != 64 || item.Subject == "" || item.PEM != ca {
		t.Fatalf("CA incorrecta: %+v", item)
	}
	if _, e = ParseRoot([]byte(rootForTest(t, false))); e == nil {
		t.Fatal("certificado leaf aceptado como CA")
	}
	if _, e = ParseRoot([]byte(ca + "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----")); e == nil {
		t.Fatal("se aceptó clave privada")
	}
	if _, e = ParseRoot([]byte(strings.Repeat("A", maxPEMSize+1))); e == nil {
		t.Fatal("no se limitó tamaño")
	}
}
func TestStorePendingApprovalAndPersistence(t *testing.T) {
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	item, e := ParseRoot([]byte(rootForTest(t, true)))
	if e != nil {
		t.Fatal(e)
	}
	item.Name = "gateway de prueba"
	if e = store.Put(item); e != nil {
		t.Fatal(e)
	}
	list, e := store.List()
	if e != nil || len(list) != 1 || list[0].Approved {
		t.Fatalf("no permanece pendiente: %#v, %v", list, e)
	}
	if e = store.Put(Anchor{Name: "cambiado", Fingerprint: strings.Repeat("0", 64), PEM: item.PEM}); e == nil {
		t.Fatal("huella maliciosa aceptada")
	}
	if _, e = store.Get("../../ca.key"); e == nil {
		t.Fatal("acepta path traversal")
	}
	item.Approved = true
	if e = store.Put(item); e != nil {
		t.Fatal(e)
	}
	recovered, e := store.Get(item.Fingerprint)
	if e != nil || !recovered.Approved {
		t.Fatal("aprobación no persistida", e)
	}
	info, e := os.Stat(store.dir)
	if e != nil || !platformfs.PrivateDirectory(info) {
		t.Fatal("permisos de dir incorrectos", e)
	}
	path, _ := store.path(item.Fingerprint)
	info, e = os.Stat(path)
	if e != nil || !platformfs.PrivateFile(info) {
		t.Fatal("permisos del inventario incorrectos", e)
	}
	if e = store.Delete(item.Fingerprint); e != nil {
		t.Fatal(e)
	}
	list, e = store.List()
	if e != nil || len(list) != 0 {
		t.Fatal("revocación/eliminación falló", e)
	}
}
func TestSyncRejectsUnapprovedAndInvalidURL(t *testing.T) {
	root, e := ParseRoot([]byte(rootForTest(t, true)))
	if e != nil {
		t.Fatal(e)
	}
	root.Endpoint = "https://127.0.0.1"
	if _, e = Sync(context.Background(), root); e == nil {
		t.Fatal("sin aprobación sincronizó")
	}
	root.Approved = true
	if _, e = Sync(context.Background(), root); e == nil {
		t.Fatal("permitió loopback")
	}
	for _, value := range []string{"http://example.org", "https://user:pass@foo", "https://foo/bar", "https://foo:65536", "https://foo?token=x"} {
		if _, e = PeerURL(value); e == nil {
			t.Fatalf("url insegura: %s", value)
		}
	}
}

func TestSyncCannotResurrectRevokedTrust(t *testing.T) {
	store, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	item, e := ParseRoot([]byte(rootForTest(t, true)))
	if e != nil {
		t.Fatal(e)
	}
	item.Name = "gateway-test"
	item.Approved = true
	item.Endpoint = "https://example.intra"
	if e = store.Put(item); e != nil {
		t.Fatal(e)
	}
	// La petición de red puede seguir activa cuando un administrador revoca la confianza.
	item.Approved = false
	if e = store.Put(item); e != nil {
		t.Fatal(e)
	}
	if e = store.MarkVerified(item.Fingerprint, item.Endpoint, time.Now()); e == nil {
		t.Fatal("un proceso de sync pudo revivir una CA revocada")
	}
	stored, e := store.Get(item.Fingerprint)
	if e != nil || stored.Approved {
		t.Fatal("CA revocada reaprobada", e)
	}
}
