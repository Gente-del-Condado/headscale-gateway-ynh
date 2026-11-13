//go:build !legacy_sqlite

package state

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type TokenInfo struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Role    string   `json:"role"`
	Scopes  []string `json:"scopes"`
	Created string   `json:"created"`
	Expires string   `json:"expires"`
}

func tokenDigest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
func (s *Store) NewToken(label, role string, scopes []string, validFor time.Duration) (string, TokenInfo, error) {
	if strings.TrimSpace(label) == "" || len(label) > 80 || validFor <= 0 || validFor > 365*24*time.Hour {
		return "", TokenInfo{}, errors.New("nombre o caducidad inválidos")
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", TokenInfo{}, e
	}
	token := "hgk_" + hex.EncodeToString(b)
	digest := tokenDigest(token)
	now := time.Now().UTC()
	info := TokenInfo{ID: digest[:16], Label: label, Role: role, Scopes: scopes, Created: now.Format(time.RFC3339), Expires: now.Add(validFor).Format(time.RFC3339)}
	packed, e := json.Marshal(scopes)
	if e != nil {
		return "", TokenInfo{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e = s.db.Exec("INSERT INTO api_tokens(hash,label,role,scopes,created_at,expires_at) VALUES(?,?,?,?,?,?)", digest, label, role, string(packed), info.Created, info.Expires)
	if e != nil {
		return "", TokenInfo{}, e
	}
	return token, info, nil
}
func (s *Store) ResolveToken(secret string) (TokenInfo, bool, error) {
	if !strings.HasPrefix(secret, "hgk_") || len(secret) != 68 {
		return TokenInfo{}, false, nil
	}
	digest := tokenDigest(secret)
	s.mu.Lock()
	defer s.mu.Unlock()
	var label, role, scopes, created, expires string
	e := s.db.QueryRow("SELECT label,role,scopes,created_at,expires_at FROM api_tokens WHERE hash=?", digest).Scan(&label, &role, &scopes, &created, &expires)
	if errors.Is(e, sql.ErrNoRows) {
		return TokenInfo{}, false, nil
	}
	if e != nil {
		return TokenInfo{}, false, e
	}
	result := TokenInfo{ID: digest[:16], Label: label, Role: role, Created: created, Expires: expires}
	if e = json.Unmarshal([]byte(scopes), &result.Scopes); e != nil {
		return TokenInfo{}, false, e
	}
	expiry, e := time.Parse(time.RFC3339, result.Expires)
	if e != nil {
		return TokenInfo{}, false, e
	}
	return result, time.Now().Before(expiry), nil
}
func (s *Store) ListTokens() ([]TokenInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT hash,label,role,scopes,created_at,expires_at FROM api_tokens ORDER BY created_at DESC")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TokenInfo{}
	for rows.Next() {
		var digest, label, role, scopes, created, expires string
		if e = rows.Scan(&digest, &label, &role, &scopes, &created, &expires); e != nil {
			return nil, e
		}
		item := TokenInfo{ID: digest[:16], Label: label, Role: role, Created: created, Expires: expires}
		if e = json.Unmarshal([]byte(scopes), &item.Scopes); e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Store) RevokeToken(id string) error {
	if len(id) != 16 {
		return errors.New("identificador de token inválido")
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return errors.New("identificador inválido")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e := s.db.Exec("DELETE FROM api_tokens WHERE substr(hash,1,16)=?", id)
	return e
}
