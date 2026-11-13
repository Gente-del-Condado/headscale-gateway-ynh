//go:build darwin

package pki

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const systemKeychain = "/Library/Keychains/System.keychain"

func SystemPlatform() string { return "darwin" }
func Instructions(a Anchor) string {
	return "macOS: requiere administrador para el llavero System. Comprueba la huella SHA-256 antes de instalar: " + a.Fingerprint
}
func Install(ctx context.Context, a Anchor) error {
	checked, err := ParseRoot([]byte(a.PEM))
	if err != nil {
		return err
	}
	if checked.Fingerprint != a.Fingerprint || !a.Approved {
		return errors.New("CA no aprobada o huella incorrecta")
	}
	if os.Geteuid() != 0 {
		return errors.New("la instalación en el llavero del sistema requiere privilegios de administrador")
	}
	tmp, err := os.CreateTemp("", "hgw-ca-*.crt")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.WriteString(checked.PEM); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", systemKeychain, tmp.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-trusted-cert: %v: %s", err, string(output))
	}
	return nil
}
func Uninstall(ctx context.Context, a Anchor) error {
	checked, err := ParseRoot([]byte(a.PEM))
	if err != nil {
		return err
	}
	if checked.Fingerprint != a.Fingerprint {
		return errors.New("huella incorrecta")
	}
	if os.Geteuid() != 0 {
		return errors.New("la retirada del llavero del sistema requiere privilegios de administrador")
	}
	block, _ := pem.Decode([]byte(checked.PEM))
	if block == nil {
		return errors.New("certificado PEM inválido")
	}
	// security usa SHA-1 solo para localizar el certificado, nunca para validarlo.
	sum := sha1.Sum(block.Bytes)
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "delete-certificate", "-Z", hex.EncodeToString(sum[:]), systemKeychain).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security delete-certificate: %v: %s", err, string(output))
	}
	return nil
}

func RemovalInstructions(a Anchor) string {
	block, _ := pem.Decode([]byte(a.PEM))
	if block == nil {
		return "CA PEM inválida"
	}
	sum := sha1.Sum(block.Bytes)
	return "sudo security delete-certificate -Z " + hex.EncodeToString(sum[:]) + " " + systemKeychain + ". SHA-256 de la CA: " + a.Fingerprint
}
