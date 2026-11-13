//go:build darwin

package pki

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func elevateHelper(ctx context.Context, path string, args []string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	command := quote(path)
	for _, a := range args {
		command += " " + quote(a)
	}
	script := "do shell script " + strconv.Quote(command) + " with administrator privileges"
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("autorización macOS cancelada u operación fallida: %w: %s", err, out)
	}
	return nil
}
