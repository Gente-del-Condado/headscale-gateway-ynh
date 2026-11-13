// Herramienta local del emisor: la clave de firma nunca se entrega a clientes ni gateways.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/licensing"
	"os"
	"path/filepath"
)

func writeNew(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: license-admin keygen --dir PRIVATE_DIR | sign --key FILE --claims FILE --out FILE")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	dir := fs.String("dir", "", "private issuer directory")
	keyPath := fs.String("key", "", "issuer private key file")
	claimsPath := fs.String("claims", "", "claims JSON file")
	out := fs.String("out", "", "new output license file")
	if e := fs.Parse(os.Args[2:]); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	switch os.Args[1] {
	case "keygen":
		if *dir == "" {
			return fmt.Errorf("--dir is required")
		}
		if e := os.Mkdir(*dir, 0700); e != nil {
			return e
		}
		pub, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		if e = writeNew(filepath.Join(*dir, "issuer.key"), []byte(base64.RawURLEncoding.EncodeToString(key))); e != nil {
			return e
		}
		return writeNew(filepath.Join(*dir, "issuer.pub"), []byte(base64.RawURLEncoding.EncodeToString(pub)))
	case "sign":
		if *keyPath == "" || *claimsPath == "" || *out == "" {
			return fmt.Errorf("--key, --claims and --out are required")
		}
		raw, e := os.ReadFile(*keyPath)
		if e != nil {
			return e
		}
		key, e := base64.RawURLEncoding.DecodeString(string(raw))
		if e != nil {
			return e
		}
		raw, e = os.ReadFile(*claimsPath)
		if e != nil {
			return e
		}
		var c licensing.Claims
		if e = json.Unmarshal(raw, &c); e != nil {
			return e
		}
		token, e := licensing.Sign(c, key)
		if e != nil {
			return e
		}
		return writeNew(*out, []byte(token+"\n"))
	default:
		return fmt.Errorf("unknown command")
	}
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
