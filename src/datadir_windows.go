//go:build windows

package main

import (
	"os"
	"path/filepath"
)

// Las instalaciones existentes en ProgramData conservan su base de datos y CA.
// Las nuevas instalaciones interactivas usan un directorio por usuario; un servicio
// debe indicar --db en un directorio dedicado con ACL NTFS restrictivas.
func defaultDataDir() (string, error) {
	if root := os.Getenv("ProgramData"); root != "" {
		legacy := filepath.Join(root, "HeadscaleGateway")
		if _, err := os.Stat(filepath.Join(legacy, "gateway.sqlite")); err == nil {
			return legacy, nil
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "HeadscaleGateway"), nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "HeadscaleGateway"), nil
}
