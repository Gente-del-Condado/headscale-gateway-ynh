// Package bootstrap concentra únicamente las opciones de arranque.
// No abre SQLite, no crea sockets y no depende de Headscale/tsnet.
package bootstrap

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Options distingue comandos de mantenimiento de la ejecución normal.
// Al ampliar la CLI, añadir los argumentos aquí en vez de modificar main.
type Options struct {
	DB                 string
	Reload             bool
	ResetRecovery      bool
	Backup             string
	RestoreBackup      string
	RestartWaitPID     int
	ApplyStagedRestore string
	SetDomain          string
	SetHeadscale       string
	Help               bool
	LogFile            string
	LogMaxMB           int
	LogBackups         int
	LogLevel           string
	Debug              bool
	Trace              bool
}

// Parse valida el formato de entrada sin efectuar ninguna modificación.
// Las rutas se resuelven después, según la plataforma y el data_dir.
func Parse(args []string) (Options, error) {
	var o Options
	fs := flag.NewFlagSet("headscale-gateway", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.DB, "db", "", "Ruta de gateway.sqlite")
	fs.BoolVar(&o.Reload, "reload", false, "Solicitar recarga")
	fs.BoolVar(&o.ResetRecovery, "reset-recovery", false, "Rotar credencial de recuperación")
	fs.StringVar(&o.Backup, "backup", "", "Compatibilidad: copia sin cifrar deshabilitada")
	fs.StringVar(&o.RestoreBackup, "restore-backup", "", "Restauración offline")
	fs.IntVar(&o.RestartWaitPID, "restart-wait-pid", 0, "Interno: reinicio Windows")
	fs.StringVar(&o.ApplyStagedRestore, "apply-staged-restore", "", "Interno: restauración preparada")
	fs.StringVar(&o.SetDomain, "set-domain", "", "Dominio base")
	fs.StringVar(&o.SetHeadscale, "set-headscale", "", "URL Headscale")
	fs.BoolVar(&o.Help, "help", false, "Mostrar ayuda")
	fs.StringVar(&o.LogFile, "log-file", "", "Archivo de registro")
	fs.IntVar(&o.LogMaxMB, "log-max-mb", 10, "MiB por registro")
	fs.IntVar(&o.LogBackups, "log-backups", 5, "Copias rotadas")
	fs.StringVar(&o.LogLevel, "log-level", "", "Nivel de registro")
	fs.BoolVar(&o.Debug, "debug", false, "Nivel DEBUG")
	fs.BoolVar(&o.Trace, "trace", false, "Nivel TRACE")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Options{Help: true}, nil
		}
		return o, fmt.Errorf("opciones de arranque inválidas: %w", err)
	}
	if len(fs.Args()) > 0 {
		return o, fmt.Errorf("argumentos inesperados: %s", strings.Join(fs.Args(), " "))
	}
	if o.Backup != "" {
		return o, errors.New("--backup sin cifrado está deshabilitado: utiliza Copias de seguridad en el panel")
	}
	if o.Debug && o.Trace {
		return o, errors.New("--debug y --trace son incompatibles; elige uno")
	}
	if o.LogMaxMB < 1 || o.LogMaxMB > 1024 {
		return o, errors.New("--log-max-mb fuera del intervalo 1..1024")
	}
	if o.LogBackups < 1 || o.LogBackups > 20 {
		return o, errors.New("--log-backups fuera del intervalo 1..20")
	}
	if o.RestartWaitPID < 0 {
		return o, errors.New("--restart-wait-pid no puede ser negativo")
	}
	// Los modos de administración offline no pueden ejecutarse mezclados.
	modes := 0
	for _, active := range []bool{o.Reload, o.ResetRecovery, o.RestoreBackup != "", o.ApplyStagedRestore != ""} {
		if active {
			modes++
		}
	}
	if modes > 1 {
		return o, errors.New("no se pueden combinar operaciones de mantenimiento")
	}
	if (o.SetDomain != "" || o.SetHeadscale != "") && modes > 0 {
		return o, errors.New("no se pueden modificar los ajustes durante una operación de mantenimiento")
	}
	return o, nil
}

// EffectiveLogLevel aplica las precedencias de CLI sin leer SQLite.
func (o Options) EffectiveLogLevel() string {
	if o.Trace {
		return "trace"
	}
	if o.Debug {
		return "debug"
	}
	return o.LogLevel
}
