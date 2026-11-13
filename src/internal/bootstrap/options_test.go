package bootstrap

import (
	"strings"
	"testing"
)

func TestOpcionesCompatibles(t *testing.T) {
	o, err := Parse([]string{"--db", "/tmp/gateway.sqlite", "--log-level", "warn", "--log-max-mb", "16"})
	if err != nil || o.DB != "/tmp/gateway.sqlite" || o.EffectiveLogLevel() != "warn" || o.LogMaxMB != 16 || o.LogBackups != 5 {
		t.Fatalf("opciones: %+v / %v", o, err)
	}
	o, err = Parse([]string{"--debug"})
	if err != nil || o.EffectiveLogLevel() != "debug" {
		t.Fatalf("debug: %+v / %v", o, err)
	}
	o, err = Parse([]string{"--trace"})
	if err != nil || o.EffectiveLogLevel() != "trace" {
		t.Fatalf("trace: %+v / %v", o, err)
	}
}

func TestRechazaConflictosYArgumentosDesconocidos(t *testing.T) {
	for _, args := range [][]string{
		{"--backup", "/tmp/backup-sin-cifrar"}, {"--trace", "--debug"}, {"--log-max-mb", "0"}, {"--log-backups", "21"},
		{"--reload", "--reset-recovery"}, {"--set-domain", "x", "--restore-backup", "y"},
		{"--desconocido"}, {"argumento-invalido"}, {"--restart-wait-pid", "-3"},
	} {
		if _, err := Parse(args); err == nil {
			t.Errorf("opciones peligrosas aceptadas: %v", args)
		}
	}
}

func TestAyudaYModoSinEstado(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		o, err := Parse([]string{arg})
		if err != nil || !o.Help {
			t.Fatalf("ayuda %q: %+v / %v", arg, o, err)
		}
	}
	o, err := Parse(nil)
	if err != nil || o.DB != "" {
		t.Fatalf("valores iniciales: %+v / %v", o, err)
	}
	if _, err := Parse([]string{"--db", strings.Repeat("a", 1024)}); err != nil {
		t.Fatal(err)
	}
}
