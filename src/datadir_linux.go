//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// Para procesos de usuario se respeta XDG_DATA_HOME; los servicios root
// siguen utilizando /var/lib/headscale-gateway.
func defaultDataDir() (string, error) {
	if os.Geteuid() == 0 {
		return "/var/lib/headscale-gateway", nil
	}
	if value := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(value) {
		return filepath.Join(value, "headscale-gateway"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "headscale-gateway"), nil
}
