//go:build windows

package pki

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

func elevateHelper(ctx context.Context, path string, args []string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	quoted := []string{}
	for _, a := range args {
		quoted = append(quoted, quote(a))
	}
	script := "$ErrorActionPreference='Stop'; try { $p=Start-Process -FilePath " + quote(path) + " -ArgumentList @(" + strings.Join(quoted, ",") + ") -Verb RunAs -Wait -PassThru; exit $p.ExitCode } catch { exit 1 }"
	units := utf16.Encode([]rune(script))
	raw := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(raw[i*2:], u)
	}
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, filepath.Join(dir, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("autorización UAC cancelada u operación fallida: %w: %s", err, out)
	}
	return nil
}
