// El paquete licensing valida suscripciones firmadas independientes del proveedor.
// Los binarios entregados solo incorporan la clave pública de verificación.
package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PublicKey se fija al compilar con -ldflags -X. Nunca acepta una clave de
// verificación desde la licencia, el entorno, HTTP ni ajustes locales.
var PublicKey string

const Product = "headscale-gateway"
const MaxSize = 16 << 10

type Claims struct {
	Version    int      `json:"version"`
	Product    string   `json:"product"`
	LicenseID  string   `json:"license_id"`
	CustomerID string   `json:"customer_id"`
	InstanceID string   `json:"instance_id"`
	NotBefore  int64    `json:"not_before"`
	ExpiresAt  int64    `json:"expires_at"`
	GraceUntil int64    `json:"grace_until"`
	Features   []string `json:"features"`
}

type Status struct {
	Enabled    bool   `json:"enabled"`
	State      string `json:"state"`
	InstanceID string `json:"instance_id,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	GraceUntil int64  `json:"grace_until,omitempty"`
	Allowed    bool   `json:"allowed"`
}

func Verify(token string, key ed25519.PublicKey, instance string, now time.Time) (Claims, string, error) {
	var c Claims
	if len(token) > MaxSize || len(key) != ed25519.PublicKeySize {
		return c, "invalid", errors.New("invalid license or verification key")
	}
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return c, "invalid", errors.New("invalid license envelope")
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return c, "invalid", e
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return c, "invalid", e
	}
	if !ed25519.Verify(key, payload, signature) {
		return c, "invalid", errors.New("invalid license signature")
	}
	if json.Unmarshal(payload, &c) != nil || c.Version != 1 || c.Product != Product || c.InstanceID != instance || instance == "" || c.LicenseID == "" || c.CustomerID == "" || c.NotBefore <= 0 || c.ExpiresAt <= c.NotBefore || c.GraceUntil < c.ExpiresAt || c.GraceUntil-c.ExpiresAt > 7*24*3600 {
		return Claims{}, "invalid", errors.New("invalid license claims")
	}
	if now.Unix() < c.NotBefore {
		return c, "not_yet_valid", errors.New("license not yet valid")
	}
	if now.Unix() >= c.GraceUntil {
		return c, "expired", errors.New("license expired")
	}
	found := false
	for _, feature := range c.Features {
		if feature == "gateway" {
			found = true
		}
	}
	if !found {
		return c, "missing_feature", errors.New("gateway feature not licensed")
	}
	if now.Unix() >= c.ExpiresAt {
		return c, "grace", nil
	}
	return c, "active", nil
}

func Sign(c Claims, key ed25519.PrivateKey) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", errors.New("invalid signing key")
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, payload))
	if _, _, err := Verify(token, key.Public().(ed25519.PublicKey), c.InstanceID, time.Unix(c.NotBefore, 0)); err != nil {
		return "", err
	}
	return token, nil
}

type Manager struct {
	mu        sync.Mutex
	directory string
	cached    Status
	checked   time.Time
}

func New(directory string) *Manager { return &Manager{directory: directory} }

func boundedRead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if len(data) > MaxSize {
		return nil, errors.New("license file too large")
	}
	return data, err
}
func Instance(directory string) (string, error) {
	path := filepath.Join(directory, "license-instance.id")
	if data, err := boundedRead(path); err == nil {
		id := strings.TrimSpace(string(data))
		raw, e := hex.DecodeString(id)
		if e != nil || len(raw) != 32 {
			return "", errors.New("invalid installation identity")
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return Instance(directory)
	}
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(id + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return id, err
}
func (m *Manager) Status() Status {
	if !Enabled {
		return Status{State: "disabled", Allowed: true}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if !m.checked.IsZero() && now.Sub(m.checked) >= 0 && now.Sub(m.checked) < 5*time.Second {
		return m.cached
	}
	result := Status{Enabled: true, State: "configuration_error"}
	defer func() { m.cached = result; m.checked = now }()
	id, err := Instance(m.directory)
	if err != nil {
		return result
	}
	result.InstanceID = id
	key, err := base64.RawURLEncoding.DecodeString(PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return result
	}
	raw, err := boundedRead(filepath.Join(m.directory, "subscription.license"))
	if err != nil {
		result.State = "missing"
		return result
	}
	c, state, err := Verify(string(raw), key, id, now)
	result.State = state
	result.Allowed = err == nil
	if c.InstanceID == id {
		result.ExpiresAt = c.ExpiresAt
		result.GraceUntil = c.GraceUntil
	}
	return result
}
