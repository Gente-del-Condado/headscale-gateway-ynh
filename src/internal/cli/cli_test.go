package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsReadOnly(t *testing.T) {
	dir := t.TempDir()
	logdir := filepath.Join(dir, "logs")
	if err := os.Mkdir(logdir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logdir, "proxy.log"), []byte("test-1\ntest-2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Execute([]string{"logs", "tail", "--dir", logdir, "--channel", "proxy", "--lines", "1"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "test-2") || strings.Contains(out.String(), "test-1") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err = Execute([]string{"doctor", "--db", filepath.Join(dir, "gateway.sqlite")}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No existe aún") {
		t.Fatal(out.String())
	}
	if err = Execute([]string{"logs", "tail", "--dir", logdir, "--channel", "../../etc"}, &out); err == nil {
		t.Fatal("acepta ruta arbitraria")
	}
	if err = Execute([]string{"logs", "tail", "--dir", logdir, "--lines", "300"}, &out); err == nil {
		t.Fatal("acepta 300 líneas")
	}
}
