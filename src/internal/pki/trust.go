// Package pki manages public trust anchors for independently operated Gateways.
// It never reads, transfers or installs a CA's private key.
package pki

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxPEMSize = 64 << 10

var fingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Anchor se limita deliberadamente a material público del certificado.
type Anchor struct {
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	Subject     string    `json:"subject"`
	Expires     time.Time `json:"expires"`
	PEM         string    `json:"pem,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
	Approved    bool      `json:"approved"`
	LastSync    time.Time `json:"last_sync,omitempty"`
}

func ParseRoot(data []byte) (Anchor, error) {
	if len(data) == 0 || len(data) > maxPEMSize {
		return Anchor{}, errors.New("certificado CA vacío o demasiado grande")
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		return Anchor{}, errors.New("se requiere un único certificado PEM, sin claves privadas")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Anchor{}, fmt.Errorf("certificado inválido: %w", err)
	}
	if !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return Anchor{}, errors.New("no es una autoridad certificadora válida")
	}
	if cert.CheckSignatureFrom(cert) != nil {
		return Anchor{}, errors.New("solo se admiten CA raíz autofirmadas; una CA intermedia necesita su cadena verificada")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return Anchor{}, errors.New("certificado de CA caducado o aún no válido")
	}
	switch key := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 {
			return Anchor{}, errors.New("clave RSA inferior a 2048 bits")
		}
	case *ecdsa.PublicKey:
		if key.Curve.Params().BitSize < 256 {
			return Anchor{}, errors.New("curva ECDSA demasiado pequeña")
		}
	case ed25519.PublicKey: // permitted
	default:
		return Anchor{}, errors.New("algoritmo de clave CA no admitido")
	}
	sum := sha256.Sum256(cert.Raw)
	return Anchor{Fingerprint: hex.EncodeToString(sum[:]), Subject: cert.Subject.String(), Expires: cert.NotAfter.UTC(), PEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))}, nil
}

var trustStoreMu sync.Mutex

type Store struct {
	dir string
	mu  *sync.Mutex
}

func NewStore(dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, errors.New("data_dir vacío")
	}
	dir := filepath.Join(dataDir, "pki", "trust")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, mu: &trustStoreMu}, nil
}
func (s *Store) path(fp string) (string, error) {
	if !fingerprintPattern.MatchString(fp) {
		return "", errors.New("huella SHA-256 no válida")
	}
	return filepath.Join(s.dir, fp+".json"), nil
}
func (s *Store) List() ([]Anchor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := []Anchor{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fp := strings.TrimSuffix(entry.Name(), ".json")
		path, e := s.path(fp)
		if e != nil {
			continue
		}
		var anchor Anchor
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &anchor); e != nil {
			return nil, e
		}
		parsed, e := ParseRoot([]byte(anchor.PEM))
		if e != nil {
			return nil, fmt.Errorf("CA %s: %w", fp, e)
		}
		if parsed.Fingerprint != fp || anchor.Fingerprint != fp {
			return nil, fmt.Errorf("CA %s: huella incorrecta", fp)
		}
		out = append(out, anchor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (s *Store) Get(fp string) (Anchor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, e := s.path(fp)
	if e != nil {
		return Anchor{}, e
	}
	content, e := os.ReadFile(path)
	if e != nil {
		return Anchor{}, e
	}
	var a Anchor
	if e = json.Unmarshal(content, &a); e != nil {
		return Anchor{}, e
	}
	checked, e := ParseRoot([]byte(a.PEM))
	if e != nil {
		return Anchor{}, e
	}
	if checked.Fingerprint != fp || a.Fingerprint != fp {
		return Anchor{}, errors.New("huella de CA no coincide")
	}
	return a, nil
}
func (s *Store) Put(a Anchor) error {
	checked, e := ParseRoot([]byte(a.PEM))
	if e != nil {
		return e
	}
	if a.Fingerprint != "" && a.Fingerprint != checked.Fingerprint {
		return errors.New("la huella declarada no coincide con el certificado")
	}
	a.Fingerprint = checked.Fingerprint
	a.PEM = checked.PEM
	a.Subject = checked.Subject
	a.Expires = checked.Expires
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" || len(a.Name) > 120 {
		return errors.New("nombre de Gateway inválido")
	}
	if len(a.Endpoint) > 1024 {
		return errors.New("URL demasiado larga")
	}
	data, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path, e := s.path(a.Fingerprint)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.dir, ".ca-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e != nil {
		tmp.Close()
		return e
	}
	if _, e = tmp.Write(data); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	return os.Rename(tmp.Name(), path)
}
func (s *Store) Delete(fp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, e := s.path(fp)
	if e != nil {
		return e
	}
	if e = os.Remove(path); errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	return e
}

// MarkVerified registra la observación sin sobrescribir la autorización.
// Si un administrador revoca un nodo durante una comprobación TLS lenta,
// el resultado antiguo nunca debe volver a aprobar la autoridad revocada.
func (s *Store) MarkVerified(fp, originalEndpoint string, verifiedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(fp)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var peer Anchor
	if err = json.Unmarshal(data, &peer); err != nil {
		return err
	}
	if peer.Fingerprint != fp || !peer.Approved || peer.Endpoint != originalEndpoint {
		return errors.New("la autorización o URL del peer cambió durante la sincronización")
	}
	check, err := ParseRoot([]byte(peer.PEM))
	if err != nil {
		return err
	}
	if check.Fingerprint != fp {
		return errors.New("la CA local cambió durante la sincronización")
	}
	peer.LastSync = verifiedAt.UTC()
	update, err := json.MarshalIndent(peer, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".sync-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(update); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ImportPending never overwrites existing approval, metadata or endpoint.
func (s *Store) ImportPending(a Anchor) (Anchor, error) {
	checked, err := ParseRoot([]byte(a.PEM))
	if err != nil {
		return Anchor{}, err
	}
	if _, err = PeerURL(a.Endpoint); err != nil {
		return Anchor{}, err
	}
	checked.Name = strings.TrimSpace(a.Name)
	if checked.Name == "" || len(checked.Name) > 120 {
		return Anchor{}, errors.New("nombre de Gateway inválido")
	}
	checked.Endpoint = a.Endpoint
	checked.Approved = false
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(checked.Fingerprint)
	if err != nil {
		return Anchor{}, err
	}
	if raw, e := os.ReadFile(path); e == nil {
		var existing Anchor
		if e = json.Unmarshal(raw, &existing); e != nil {
			return Anchor{}, e
		}
		return existing, nil
	} else if !os.IsNotExist(e) {
		return Anchor{}, e
	}
	raw, _ := json.MarshalIndent(checked, "", "  ")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Anchor{}, err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return Anchor{}, err
	}
	return checked, nil
}
