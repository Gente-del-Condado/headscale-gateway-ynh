// El paquete securebackup crea archivos portables recuperables con contraseña y
// cifrables con clave pública. La copia desatendida solo necesita material público.
package securebackup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const magic = "HGWBKP02"
const chunkSize = 64 * 1024
const rounds = 350000

// KeyFile puede copiarse fuera del equipo; su clave privada está cifrada con contraseña.
type KeyFile struct {
	Version          int    `json:"version"`
	Iterations       int    `json:"iterations"`
	Public           []byte `json:"public"`
	Salt             []byte `json:"salt"`
	Nonce            []byte `json:"nonce"`
	PrivateEncrypted []byte `json:"private_encrypted"`
}

func kdf(password string, salt []byte, iterations int) []byte {
	// PBKDF2-HMAC-SHA256, dkLen=32; la contraseña nunca se guarda.
	var block [4]byte
	binary.BigEndian.PutUint32(block[:], 1)
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(salt)
	mac.Write(block[:])
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		m := hmac.New(sha256.New, []byte(password))
		m.Write(u)
		u = m.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
func seal(key, nonce, plaintext, aad []byte) ([]byte, error) {
	a, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(a)
	if e != nil {
		return nil, e
	}
	return g.Seal(nil, nonce, plaintext, aad), nil
}
func unseal(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	a, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(a)
	if e != nil {
		return nil, e
	}
	return g.Open(nil, nonce, ciphertext, aad)
}

// GenerateKey crea un par X25519 efímero cuya parte privada queda protegida
// por la contraseña. No persiste secretos ni depende del sistema operativo.
func GenerateKey(password string) (KeyFile, error) {
	if len(password) < 12 {
		return KeyFile{}, errors.New("la contraseña debe tener al menos 12 caracteres")
	}
	pair, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return KeyFile{}, err
	}
	k := KeyFile{Version: 2, Iterations: rounds, Public: pair.PublicKey().Bytes(), Salt: make([]byte, 16), Nonce: make([]byte, 12)}
	if _, err = rand.Read(k.Salt); err != nil {
		return KeyFile{}, err
	}
	if _, err = rand.Read(k.Nonce); err != nil {
		return KeyFile{}, err
	}
	k.PrivateEncrypted, err = seal(kdf(password, k.Salt, k.Iterations), k.Nonce, pair.Bytes(), []byte(magic))
	if err != nil {
		return KeyFile{}, err
	}
	return k, nil
}

// NewKey guarda el par cifrado en un fichero privado de forma exclusiva.
func NewKey(path, password string) error {
	k, err := GenerateKey(password)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(k)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
func ReadKey(path string) (KeyFile, error) {
	var k KeyFile
	raw, e := os.ReadFile(path)
	if e != nil {
		return k, e
	}
	e = json.Unmarshal(raw, &k)
	if e != nil {
		return k, e
	}
	if k.Version != 2 || k.Iterations < 300000 || k.Iterations > 3000000 || len(k.Public) != 32 || len(k.Salt) != 16 || len(k.Nonce) != 12 || len(k.PrivateEncrypted) != 48 {
		return k, errors.New("clave de respaldo incompatible")
	}
	return k, nil
}
func Unlock(k KeyFile, password string) (*ecdh.PrivateKey, error) {
	plain, e := unseal(kdf(password, k.Salt, k.Iterations), k.Nonce, k.PrivateEncrypted, []byte(magic))
	if e != nil {
		return nil, errors.New("contraseña incorrecta o archivo alterado")
	}
	priv, e := ecdh.X25519().NewPrivateKey(plain)
	if e != nil {
		return nil, e
	}
	if !bytes.Equal(priv.PublicKey().Bytes(), k.Public) {
		return nil, errors.New("clave pública no corresponde")
	}
	return priv, nil
}
func CheckPassword(path, password string) error {
	k, e := ReadKey(path)
	if e != nil {
		return e
	}
	_, e = Unlock(k, password)
	return e
}
func derive(shared []byte) []byte {
	m := hmac.New(sha256.New, []byte("hgw secure backup aes-gcm v2"))
	m.Write(shared)
	return m.Sum(nil)
}
func readExact(r io.Reader, n uint32) ([]byte, error) {
	if n > 16384 {
		return nil, errors.New("cabecera demasiado grande")
	}
	b := make([]byte, n)
	_, e := io.ReadFull(r, b)
	return b, e
}
func writeHeader(w io.Writer, k KeyFile, ephemeral []byte, noncePrefix []byte) ([]byte, error) {
	header, e := json.Marshal(struct {
		Key         KeyFile `json:"key"`
		Ephemeral   []byte  `json:"ephemeral"`
		NoncePrefix []byte  `json:"nonce_prefix"`
	}{k, ephemeral, noncePrefix})
	if e != nil {
		return nil, e
	}
	if len(header) > 16384 {
		return nil, errors.New("cabecera excesiva")
	}
	if _, e = w.Write([]byte(magic)); e != nil {
		return nil, e
	}
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(header)))
	if _, e = w.Write(l[:]); e != nil {
		return nil, e
	}
	_, e = w.Write(header)
	return append(append([]byte(magic), l[:]...), header...), e
}
func readHeader(r io.Reader) (KeyFile, []byte, []byte, []byte, error) {
	var k KeyFile
	var m [8]byte
	if _, e := io.ReadFull(r, m[:]); e != nil {
		return k, nil, nil, nil, e
	}
	if string(m[:]) != magic {
		return k, nil, nil, nil, errors.New("no es una copia cifrada HGW v2")
	}
	var lenB [4]byte
	if _, e := io.ReadFull(r, lenB[:]); e != nil {
		return k, nil, nil, nil, e
	}
	raw, e := readExact(r, binary.BigEndian.Uint32(lenB[:]))
	if e != nil {
		return k, nil, nil, nil, e
	}
	var h struct {
		Key         KeyFile `json:"key"`
		Ephemeral   []byte  `json:"ephemeral"`
		NoncePrefix []byte  `json:"nonce_prefix"`
	}
	if e = json.Unmarshal(raw, &h); e != nil {
		return k, nil, nil, nil, e
	}
	if h.Key.Version != 2 || h.Key.Iterations < 300000 || h.Key.Iterations > 3000000 || len(h.Key.Public) != 32 || len(h.Key.Salt) != 16 || len(h.Key.Nonce) != 12 || len(h.Ephemeral) != 32 || len(h.NoncePrefix) != 8 {
		return k, nil, nil, nil, errors.New("cabecera cifrada inválida")
	}
	return h.Key, h.Ephemeral, h.NoncePrefix, append(append(m[:], lenB[:]...), raw...), nil
}
func aead(shared []byte) (cipher.AEAD, error) {
	block, e := aes.NewCipher(derive(shared))
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func iv(prefix []byte, counter uint32) []byte {
	b := make([]byte, 12)
	copy(b, prefix)
	binary.BigEndian.PutUint32(b[8:], counter)
	return b
}
func aad(header []byte, counter uint32) []byte {
	h := sha256.Sum256(header)
	out := make([]byte, 36)
	copy(out, h[:])
	binary.BigEndian.PutUint32(out[32:], counter)
	return out
}

type encryptedWriter struct {
	w       io.Writer
	g       cipher.AEAD
	prefix  []byte
	header  []byte
	buf     []byte
	counter uint32
	closed  bool
}

func NewWriter(w io.Writer, k KeyFile) (io.WriteCloser, error) {
	pub, e := ecdh.X25519().NewPublicKey(k.Public)
	if e != nil {
		return nil, e
	}
	ep, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		return nil, e
	}
	shared, e := ep.ECDH(pub)
	if e != nil {
		return nil, e
	}
	g, e := aead(shared)
	if e != nil {
		return nil, e
	}
	prefix := make([]byte, 8)
	if _, e = rand.Read(prefix); e != nil {
		return nil, e
	}
	header, e := writeHeader(w, k, ep.PublicKey().Bytes(), prefix)
	if e != nil {
		return nil, e
	}
	return &encryptedWriter{w: w, g: g, prefix: prefix, header: header, buf: make([]byte, 0, chunkSize)}, nil
}
func (w *encryptedWriter) flush(final bool) error {
	if w.counter == ^uint32(0) {
		return errors.New("archivo cifrado demasiado grande")
	}
	if len(w.buf) == 0 && !final {
		return nil
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(w.buf)))
	if _, e := w.w.Write(size[:]); e != nil {
		return e
	}
	ct := w.g.Seal(nil, iv(w.prefix, w.counter), w.buf, aad(w.header, w.counter))
	if _, e := w.w.Write(ct); e != nil {
		return e
	}
	w.counter++
	w.buf = w.buf[:0]
	return nil
}
func (w *encryptedWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("archivo cerrado")
	}
	n := len(p)
	for len(p) > 0 {
		space := chunkSize - len(w.buf)
		if space > len(p) {
			space = len(p)
		}
		w.buf = append(w.buf, p[:space]...)
		p = p[space:]
		if len(w.buf) == chunkSize {
			if e := w.flush(false); e != nil {
				return n - len(p), e
			}
		}
	}
	return n, nil
}
func (w *encryptedWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if e := w.flush(false); e != nil {
		return e
	}
	return w.flush(true)
}

type encryptedReader struct {
	r              io.Reader
	g              cipher.AEAD
	prefix, header []byte
	counter        uint32
	buf            []byte
	end            bool
}

func NewReader(r io.Reader, password string) (io.Reader, error) {
	k, ep, prefix, header, e := readHeader(r)
	if e != nil {
		return nil, e
	}
	priv, e := Unlock(k, password)
	if e != nil {
		return nil, e
	}
	pub, e := ecdh.X25519().NewPublicKey(ep)
	if e != nil {
		return nil, e
	}
	shared, e := priv.ECDH(pub)
	if e != nil {
		return nil, e
	}
	g, e := aead(shared)
	if e != nil {
		return nil, e
	}
	return &encryptedReader{r: r, g: g, prefix: prefix, header: header}, nil
}
func (r *encryptedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.buf) == 0 && !r.end {
		if r.counter == ^uint32(0) {
			return 0, errors.New("archivo cifrado demasiado grande")
		}
		var size [4]byte
		if _, e := io.ReadFull(r.r, size[:]); e != nil {
			return 0, fmt.Errorf("copia truncada: %w", e)
		}
		n := binary.BigEndian.Uint32(size[:])
		if n > chunkSize {
			return 0, errors.New("bloque cifrado excesivo")
		}
		ciphertext := make([]byte, int(n)+r.g.Overhead())
		if _, e := io.ReadFull(r.r, ciphertext); e != nil {
			return 0, fmt.Errorf("copia truncada: %w", e)
		}
		plaintext, e := r.g.Open(nil, iv(r.prefix, r.counter), ciphertext, aad(r.header, r.counter))
		if e != nil {
			return 0, errors.New("fallo de integridad o archivo manipulado")
		}
		r.counter++
		if n == 0 {
			var check [1]byte
			if nr, er := r.r.Read(check[:]); nr != 0 || er != io.EOF {
				return 0, errors.New("datos inesperados al final")
			}
			r.end = true
			break
		}
		r.buf = plaintext
	}
	if r.end && len(r.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// HasEncryptedHeader reconoce el contenedor. Para autenticar su contenido se
// necesita la contraseña y leerlo completo con NewReader.
func HasEncryptedHeader(r io.Reader) bool {
	_, _, _, _, err := readHeader(r)
	return err == nil
}
