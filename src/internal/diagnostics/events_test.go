package diagnostics

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitEventsAllChannels(t *testing.T) {
	t.Setenv("GATEWAY_LANG", "es")
	defer SetLevel("info")
	SetLevel("warn")
	dir := t.TempDir()
	if err := Configure(dir, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer Close()
	for _, channel := range channelNames {
		Emit(channel, Warn, "HGW-TEST-W", "Aviso: %s", "error en detalle")
		Emit(channel, Error, "HGW-TEST-E", "Fallo\nBearer secreto")
		Emit(channel, InfoLevel, "HGW-TEST-I", "descartado")
		raw, err := os.ReadFile(filepath.Join(dir, channel+".log"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Count(text, "[ADVERTENCIA] [HGW-TEST-W]") != 1 || strings.Count(text, "[ERROR] [HGW-TEST-E]") != 2 || strings.Contains(text, "descartado") || strings.Contains(text, "secreto") {
			t.Fatal(text)
		}
	}
}
func TestExternalOutputDoesNotInferSeverity(t *testing.T) {
	t.Setenv("GATEWAY_LANG", "es")
	defer SetLevel("info")
	SetLevel("info")
	dir := t.TempDir()
	if err := Configure(dir, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer Close()
	log.Print("WARNING: technical dependency text")
	lines, err := Tail("gateway", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "[INFORMACIÓN] [HGW-EXTERNAL-STDLOG]") {
		t.Fatal(lines)
	}
}
func TestTailDoesNotRewriteHistoricalRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.log")
	os.WriteFile(path, []byte("WARNING: historical record\n"), 0600)
	lines, err := tailFile(path, 1)
	if err != nil || len(lines) != 1 || lines[0] != "WARNING: historical record" {
		t.Fatal(lines, err)
	}
}
func TestEnglishCanonicalLevels(t *testing.T) {
	t.Setenv("GATEWAY_LANG", "en")
	for i, want := range []string{"ERROR", "WARNING", "INFORMATION", "DEBUG", "TRACE"} {
		if got := Level(i).Label(); got != want {
			t.Fatalf("%s != %s", got, want)
		}
	}
}

func TestEnglishEventTemplateAndErrorDetails(t *testing.T) {
	t.Setenv("GATEWAY_LANG", "en")
	defer SetLevel("info")
	SetLevel("info")
	dir := t.TempDir()
	if err := Configure(dir, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer Close()
	Emit("gateway", InfoLevel, "HGW-TEST-EN", "registro inicializado; archivo=%s", "example.log")
	lines, err := Tail("gateway", 5)
	if err != nil || len(lines) != 1 {
		t.Fatal(lines, err)
	}
	if !strings.Contains(lines[0], "[INFORMATION] [HGW-TEST-EN] logging initialized; file=example.log") {
		t.Fatal(lines)
	}
}
