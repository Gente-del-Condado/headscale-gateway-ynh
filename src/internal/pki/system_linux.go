//go:build linux && !android

package pki

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrPrivilege = errors.New("la instalación global de CA requiere root; configura un helper privilegiado o usa el procedimiento manual")

func SystemPlatform() string { return "linux" }
func installPath(a Anchor) (string, string, []string, error) {
	if _, err := os.Stat("/etc/pki/ca-trust/source/anchors"); err == nil {
		return "/etc/pki/ca-trust/source/anchors/hgw-" + a.Fingerprint + ".crt", "update-ca-trust", []string{"extract"}, nil
	}
	if _, err := os.Stat("/usr/local/share/ca-certificates"); err == nil {
		return "/usr/local/share/ca-certificates/hgw-" + a.Fingerprint + ".crt", "update-ca-certificates", nil, nil
	}
	return "", "", nil, errors.New("almacén de CA del sistema no reconocido")
}
func Instructions(a Anchor) string {
	path, cmd, args, e := installPath(a)
	if e != nil {
		return e.Error()
	}
	return fmt.Sprintf("Necesita root. Guarda la CA pública como %s y ejecuta: sudo %s %s. Comprueba antes la huella SHA-256: %s", path, cmd, strings.Join(args, " "), a.Fingerprint)
}
func Install(ctx context.Context, a Anchor) error {
	valid, e := ParseRoot([]byte(a.PEM))
	if e != nil {
		return e
	}
	if valid.Fingerprint != a.Fingerprint || !a.Approved {
		return errors.New("CA no aprobada o huella incorrecta")
	}
	if os.Geteuid() != 0 {
		return ErrPrivilege
	}
	path, cmd, args, e := installPath(a)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(path); e == nil {
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		present, parseErr := ParseRoot(existing)
		if parseErr == nil && present.Fingerprint == a.Fingerprint {
			return nil
		}
		return errors.New("existe un ancla con este nombre pero no coincide con el certificado esperado")
	}
	if e = os.WriteFile(path, []byte(valid.PEM), 0644); e != nil {
		return e
	}
	output, e := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	if e != nil {
		_ = os.Remove(path)
		return fmt.Errorf("actualizando confianza: %v: %s", e, string(output))
	}
	return nil
}
func Uninstall(ctx context.Context, a Anchor) error {
	path, cmd, args, e := installPath(a)
	if e != nil {
		return e
	}
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if os.Geteuid() != 0 {
		return errors.New("la retirada global de CA requiere root")
	}
	checked, e := ParseRoot(data)
	if e != nil {
		return e
	}
	if checked.Fingerprint != a.Fingerprint {
		return errors.New("el certificado instalado no coincide con la huella autorizada")
	}
	if filepath.Base(path) != "hgw-"+a.Fingerprint+".crt" {
		return errors.New("ruta no gestionada")
	}
	if e = os.Remove(path); e != nil {
		return e
	}
	output, e := exec.CommandContext(ctx, cmd, args...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("actualizando confianza: %v: %s", e, string(output))
	}
	return nil
}

func RemovalInstructions(a Anchor) string {
	path, cmd, args, err := installPath(a)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("Comprueba SHA-256 %s y retira el ancla gestionada: sudo rm -- %s; sudo %s %s", a.Fingerprint, path, cmd, strings.Join(args, " "))
}
