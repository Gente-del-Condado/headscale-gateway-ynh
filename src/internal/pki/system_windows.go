//go:build windows

package pki

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func SystemPlatform() string { return "windows" }
func Instructions(a Anchor) string {
	return "Requiere PowerShell o CMD como administrador para el almacén de equipo. Comprueba la huella SHA-256: " + a.Fingerprint
}
func Install(ctx context.Context, a Anchor) error {
	checked, e := ParseRoot([]byte(a.PEM))
	if e != nil {
		return e
	}
	if checked.Fingerprint != a.Fingerprint || !a.Approved {
		return errors.New("CA no aprobada")
	}
	f, e := os.CreateTemp("", "hgw-ca-*.crt")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, e = f.WriteString(checked.PEM); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	output, e := exec.CommandContext(ctx, "certutil", "-addstore", "-f", "Root", f.Name()).CombinedOutput()
	if e != nil {
		return fmt.Errorf("certutil: %v: %s", e, string(output))
	}
	return nil
}
func checkedDER(p string) []byte {
	block, _ := pem.Decode([]byte(p))
	if block == nil {
		return nil
	}
	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return nil
	}
	return cert.Raw
}
func Uninstall(ctx context.Context, a Anchor) error {
	checked, e := ParseRoot([]byte(a.PEM))
	if e != nil {
		return e
	}
	if checked.Fingerprint != a.Fingerprint {
		return errors.New("huella incorrecta")
	}
	// certutil delstore uses Windows certificate SHA-1 thumbprint as an identifier,
	// NOT as a trust or validation primitive. Approval uses SHA-256 above.
	sum := sha1.Sum(checkedDER(a.PEM))
	thumbprint := hex.EncodeToString(sum[:])
	output, e := exec.CommandContext(ctx, "certutil", "-delstore", "Root", thumbprint).CombinedOutput()
	if e != nil {
		return fmt.Errorf("certutil: %v: %s", e, string(output))
	}
	return nil
}

func RemovalInstructions(a Anchor) string {
	sum := sha1.Sum(checkedDER(a.PEM))
	return "En una consola como administrador: certutil -delstore Root " + hex.EncodeToString(sum[:]) + ". SHA-256 de la CA: " + a.Fingerprint
}
