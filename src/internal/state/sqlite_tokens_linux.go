//go:build legacy_sqlite && linux && cgo

package state

/*
#include <sqlite3.h>
*/
import "C"

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unsafe"
)

// TokenInfo omite deliberadamente el token Bearer original y su resumen completo.

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
	if _, err := rand.Read(b); err != nil {
		return "", TokenInfo{}, err
	}
	token := "hgk_" + hex.EncodeToString(b)
	digest := tokenDigest(token)
	now := time.Now().UTC()
	info := TokenInfo{ID: digest[:16], Label: label, Role: role, Scopes: scopes, Created: now.Format(time.RFC3339), Expires: now.Add(validFor).Format(time.RFC3339)}
	packed, err := json.Marshal(scopes)
	if err != nil {
		return "", TokenInfo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("INSERT INTO api_tokens(hash,label,role,scopes,created_at,expires_at) VALUES(?,?,?,?,?,?)")
	if err != nil {
		return "", TokenInfo{}, err
	}
	defer C.sqlite3_finalize(stmt)
	for i, val := range []string{digest, label, role, string(packed), info.Created, info.Expires} {
		if err = bind(stmt, i+1, val); err != nil {
			return "", TokenInfo{}, err
		}
	}
	if C.sqlite3_step(stmt) != C.SQLITE_DONE {
		return "", TokenInfo{}, s.err()
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
	stmt, err := s.prepare("SELECT label,role,scopes,created_at,expires_at FROM api_tokens WHERE hash=?")
	if err != nil {
		return TokenInfo{}, false, err
	}
	defer C.sqlite3_finalize(stmt)
	if err = bind(stmt, 1, digest); err != nil {
		return TokenInfo{}, false, err
	}
	rc := C.sqlite3_step(stmt)
	if rc == C.SQLITE_DONE {
		return TokenInfo{}, false, nil
	}
	if rc != C.SQLITE_ROW {
		return TokenInfo{}, false, s.err()
	}
	get := func(i int) string {
		return C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, C.int(i)))))
	}
	out := TokenInfo{ID: digest[:16], Label: get(0), Role: get(1), Created: get(3), Expires: get(4)}
	if e := json.Unmarshal([]byte(get(2)), &out.Scopes); e != nil {
		return TokenInfo{}, false, e
	}
	expiry, e := time.Parse(time.RFC3339, out.Expires)
	if e != nil {
		return TokenInfo{}, false, e
	}
	return out, time.Now().Before(expiry), nil
}
func (s *Store) ListTokens() ([]TokenInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("SELECT hash,label,role,scopes,created_at,expires_at FROM api_tokens ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)
	result := []TokenInfo{}
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			return result, nil
		}
		if rc != C.SQLITE_ROW {
			return nil, s.err()
		}
		get := func(i int) string {
			return C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, C.int(i)))))
		}
		out := TokenInfo{ID: get(0)[:16], Label: get(1), Role: get(2), Created: get(4), Expires: get(5)}
		if err = json.Unmarshal([]byte(get(3)), &out.Scopes); err != nil {
			return nil, err
		}
		result = append(result, out)
	}
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
	stmt, err := s.prepare("DELETE FROM api_tokens WHERE substr(hash,1,16)=?")
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)
	if err = bind(stmt, 1, id); err != nil {
		return err
	}
	if C.sqlite3_step(stmt) != C.SQLITE_DONE {
		return s.err()
	}
	return nil
}
