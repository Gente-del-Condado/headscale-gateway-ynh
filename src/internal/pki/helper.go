package pki

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// HelperRequest acepta una acción y una única CA pública autofirmada.
// No acepta rutas de destino, comandos, claves privadas ni scripts del llamante.
func HelperRequest(args []string) (string, Anchor, error) {
	if len(args) != 3 || (args[0] != "install" && args[0] != "uninstall") {
		return "", Anchor{}, errors.New("uso: install|uninstall SHA256 certificado-PEM-base64")
	}
	if !fingerprintPattern.MatchString(args[1]) || len(args[2]) > 12000 {
		return "", Anchor{}, errors.New("huella o tamaño inválidos")
	}
	raw, err := base64.StdEncoding.DecodeString(args[2])
	if err != nil {
		return "", Anchor{}, err
	}
	a, err := ParseRoot(raw)
	if err != nil {
		return "", Anchor{}, err
	}
	if a.Fingerprint != args[1] {
		return "", Anchor{}, errors.New("la huella no coincide con la CA pública")
	}
	a.Approved = true
	return args[0], a, nil
}
func HelperPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := "hgw-ca-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}
func HelperStatus() map[string]any {
	if runtime.GOOS == "android" {
		return map[string]any{"available": false, "platform": "android", "mode": "manual-settings", "path": ""}
	}
	path, err := HelperPath()
	available := false
	if err == nil {
		if info, e := os.Stat(path); e == nil && info.Mode().IsRegular() {
			available = true
		}
	}
	return map[string]any{"available": available, "path": path, "platform": runtime.GOOS, "mode": "interactive-elevation"}
}
func RunElevatedHelper(ctx context.Context, action string, a Anchor) error {
	args := []string{action, a.Fingerprint, base64.StdEncoding.EncodeToString([]byte(a.PEM))}
	if action == "install" && !a.Approved {
		return errors.New("CA no aprobada")
	}
	if _, _, err := HelperRequest(args); err != nil {
		return err
	}
	path, err := HelperPath()
	if err != nil {
		return err
	}
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return errors.New("helper no instalado junto al ejecutable del Gateway")
	}
	if err = elevateHelper(ctx, path, args); err != nil {
		return fmt.Errorf("helper: %w; comprueba el almacén antes de repetir si se agotó el tiempo", err)
	}
	return nil
}
