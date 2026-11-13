package pki

import (
	"errors"
	"os"
	"path/filepath"
)

// RemoveLocal elimina la clave emisora y los certificados del almacén del Gateway.
// Conserva la CA pública aparte para poder reintentar su desinstalación del SO.
func RemoveLocal(dataDir, name, fingerprint string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return errors.New("nombre de CA inválido")
	}
	dir := filepath.Join(dataDir, "local-ca", name)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("directorio de CA inválido")
	}
	content, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return err
	}
	root, err := ParseRoot(content)
	if err != nil {
		return err
	}
	if root.Fingerprint != fingerprint {
		return errors.New("la CA ha cambiado; actualiza el inventario")
	}
	archive := filepath.Join(dataDir, "pki", "removed-public-ca")
	if err = os.MkdirAll(archive, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(archive, fingerprint+".crt"), []byte(root.PEM), 0600); err != nil {
		return err
	}
	// Rename first: readers never see a half-removed CA/key pair in local-ca.
	staging, err := os.MkdirTemp(filepath.Join(dataDir, "pki"), "remove-")
	if err != nil {
		return err
	}
	defer os.Remove(staging)
	retired := filepath.Join(staging, "ca")
	if err = os.Rename(dir, retired); err != nil {
		return err
	}
	return os.RemoveAll(retired)
}
