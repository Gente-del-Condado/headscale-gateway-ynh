//go:build linux && !android

package pki

import (
	"context"
	"fmt"
	"os/exec"
)

func elevateHelper(ctx context.Context, path string, args []string) error {
	out, err := exec.CommandContext(ctx, "/usr/bin/pkexec", append([]string{path}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("autorización polkit cancelada, no disponible u operación fallida: %w: %s", err, out)
	}
	return nil
}
