package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelsAndTemporaryDebug(t *testing.T) {
	defer func() { _ = SetLevel("info") }()
	if err := SetLevel("warn"); err != nil {
		t.Fatal(err)
	}
	if Enabled(Debug) || Enabled(InfoLevel) || !Enabled(Error) {
		t.Fatal("filtro de niveles incorrecto")
	}
	if _, err := EnableTemporaryDebug(65); err == nil {
		t.Fatal("duración inválida aceptada")
	}
	if _, err := EnableTemporaryDebug(1); err != nil {
		t.Fatal(err)
	}
	if !Enabled(Debug) || Enabled(Trace) {
		t.Fatal("debug temporal no aplicado")
	}
	DisableTemporaryDebug()
	if Enabled(Debug) {
		t.Fatal("debug temporal no revocado")
	}
	if err := SetLevel("invalid"); err == nil {
		t.Fatal("nivel inválido aceptado")
	}
}
func TestRedactionBeforeWriting(t *testing.T) {
	var dest bytes.Buffer
	value := "Bearer abc.DEF-123 hskey-api-AbC123 hgk_789 client_secret=supersecreto password:clave https://dns.invalid/?token=provider%2Bprivate&zone=local"
	w := redactedWriter{&dest}
	if _, err := w.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
	text := dest.String()
	for _, needle := range []string{"abc.DEF-123", "hskey-api-", "hgk_789", "supersecreto", "clave", "provider%2Bprivate"} {
		if strings.Contains(text, needle) {
			t.Fatalf("dato secreto visible: %s", needle)
		}
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatal("no se ha ocultado")
	}
}
func TestTailAtPrivateOnly(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "admin.log"), []byte("[INFO] normal\nBearer secreto\n"), 0600)
	lines, err := TailAt(dir, "admin", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || strings.Contains(lines[1], "secreto") {
		t.Fatalf("lectura no redactada: %v", lines)
	}
	if _, err := TailAt(dir, "../admin", 2); err == nil {
		t.Fatal("canal arbitrario admitido")
	}
	if err := os.Symlink(filepath.Join(dir, "admin.log"), filepath.Join(dir, "gateway.log")); err == nil {
		if _, err = TailAt(dir, "gateway", 2); err == nil {
			t.Fatal("symlink admitido")
		}
	}
}
