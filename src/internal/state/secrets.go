package state

// Los secretos persistentes se cifran con AES-256-GCM. La clave maestra reside
// fuera de SQLite: una base robada no debe contener también la clave para
// descifrar credenciales LDAP, Dex, TOTP y de los servicios de destino.
import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const encryptedPrefix = "enc:v1:"

type secretBox struct{ aead cipher.AEAD }

func openSecretBox(path string) (*secretBox, error) {
	key := make([]byte, 32)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		if _, err = rand.Read(key); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, err
		}
		if _, err = f.Write(key); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, err
		}
		if err = f.Sync(); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, err
		}
		if err = f.Close(); err != nil {
			return nil, err
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || (runtime.GOOS != "windows" && stat.Mode().Perm() != 0600) {
		return nil, fmt.Errorf("master.key requiere archivo regular con permisos 0600")
	}
	// A concurrent process may be finishing first creation; reject short keys
	// rather than silently switching to an incompatible key.
	loaded, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(loaded) != 32 {
		return nil, errors.New("master.key corrupta o incompleta")
	}
	block, err := aes.NewCipher(loaded)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretBox{aead: aead}, nil
}
func (s *secretBox) seal(clear string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	result := s.aead.Seal(nonce, nonce, []byte(clear), nil)
	return encryptedPrefix + base64.RawStdEncoding.EncodeToString(result), nil
}
func (s *secretBox) open(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, encryptedPrefix) {
		return "", errors.New("valor sin cifrar o versión no compatible")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encoded, encryptedPrefix))
	if err != nil {
		return "", err
	}
	if len(raw) < s.aead.NonceSize() {
		return "", errors.New("valor cifrado truncado")
	}
	plain, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("secreto ilegible (clave incompatible o datos modificados): %w", err)
	}
	return string(plain), nil
}
