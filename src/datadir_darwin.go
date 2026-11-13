//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

func defaultDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(
		configDir,
		"HeadscaleGateway",
	), nil
}
