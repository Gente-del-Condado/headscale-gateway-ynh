package admin

// TOTP RFC 6238 para el administrador local. Los proveedores externos (Dex/LDAP)
// deben exigir MFA en el proveedor de identidad: el segundo factor local no se
// aplica automáticamente a los accesos federados.
import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type pendingTOTP struct {
	secret  string
	expires time.Time
}

func totp(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(payload[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	val := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", val)
}
func validTOTP(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / 30
	for _, delta := range []int64{-1, 0, 1} {
		candidate := totp(secret, step+delta)
		if candidate != "" && subtle.ConstantTimeCompare([]byte(code), []byte(candidate)) == 1 {
			return step + delta, true
		}
	}
	return 0, false
}
func (a *AuthManager) MFASetup(user string) (string, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.config.Mode != "local" || user != a.config.LocalUsername {
		return "", "", errors.New("MFA local requiere sesión local del administrador")
	}
	if a.config.MFAEnabled {
		return "", "", errors.New("MFA ya está habilitado")
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	a.pendingMFA[user] = pendingTOTP{secret, time.Now().Add(10 * time.Minute)}
	issuer := "Headscale Gateway"
	uri := "otpauth://totp/" + url.PathEscape(issuer+":"+user) + "?secret=" + url.QueryEscape(secret) + "&issuer=" + url.QueryEscape(issuer) + "&algorithm=SHA1&digits=6&period=30"
	return secret, uri, nil
}
func (a *AuthManager) MFAConfirm(user, code string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	pending, ok := a.pendingMFA[user]
	if !ok || time.Now().After(pending.expires) {
		return errors.New("configuración MFA caducada")
	}
	if _, ok = validTOTP(pending.secret, code, time.Now()); !ok {
		return errors.New("código TOTP incorrecto")
	}
	c := a.config
	c.MFAEnabled = true
	c.MFASecret = pending.secret
	if err := a.saveMFALocked(c); err != nil {
		return err
	}
	delete(a.pendingMFA, user)
	return nil
}
func (a *AuthManager) VerifyLocalMFA(code string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.config.MFAEnabled {
		return true
	}
	step, valid := validTOTP(a.config.MFASecret, strings.TrimSpace(code), time.Now())
	if !valid || step <= a.usedTOTP[a.config.LocalUsername] {
		return false
	}
	a.usedTOTP[a.config.LocalUsername] = step
	return true
}
func (a *AuthManager) MFADisable(user, code string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.config.Mode != "local" || user != a.config.LocalUsername || !a.config.MFAEnabled {
		return errors.New("MFA local no está habilitado")
	}
	if _, valid := validTOTP(a.config.MFASecret, code, time.Now()); !valid {
		return errors.New("código inválido")
	}
	c := a.config
	c.MFAEnabled = false
	c.MFASecret = ""
	return a.saveMFALocked(c)
}

// Actualización atómica: el archivo temporal es privado y nunca se sirve por HTTP.
func (a *AuthManager) saveMFALocked(c AuthConfig) error {
	if a.store != nil {
		if err := a.store.PutJSON("auth_config_v1", c); err != nil {
			return err
		}
		a.config = c
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(a.path), 0700); err != nil {
		return err
	}
	name, err := randomHex(12)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(a.path), ".auth-"+name)
	if err = os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, a.path); err != nil {
		os.Remove(tmp)
		return err
	}
	a.config = c
	return nil
}
