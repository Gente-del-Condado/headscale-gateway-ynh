package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gentedelcondado/headscale-gateway/internal/bootstrap"
	"github.com/gentedelcondado/headscale-gateway/internal/cli"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func main() {
	// Los subcomandos de diagnóstico se ejecutan sin arrancar tsnet ni el panel.
	if len(os.Args) > 1 && cli.IsCommand(os.Args[1]) {
		if err := cli.Run(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("ERROR CLI:"), i18n.T(err.Error()))
			os.Exit(1)
		}
		return
	}
	opts, err := bootstrap.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T(err.Error()))
		os.Exit(2)
	}
	if opts.Help {
		printHelp()
		return
	}
	// Prioridad: opción CLI explícita > variable de entorno > SQLite > info.
	cliLevel := opts.EffectiveLogLevel()
	envLevel := os.Getenv("GATEWAY_LOG_LEVEL")
	if cliLevel != "" {
		if err := diagnostics.SetLevel(cliLevel); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T(err.Error()))
			os.Exit(2)
		}
	} else if envLevel != "" {
		if err := diagnostics.SetLevel(envLevel); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T(err.Error()))
			os.Exit(2)
		}
	}
	configPath, logFile := opts.DB, opts.LogFile
	if opts.RestartWaitPID != 0 {
		if err := waitForPreviousGateway(opts.RestartWaitPID); err != nil {
			diagnostics.Fatal("HGW-MAIN-001", "No se puede continuar: %v", err)
		}
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if configPath == "" {
		directory, err := defaultDataDir()
		if err != nil {
			diagnostics.Fatal("HGW-MAIN-002", "No se puede continuar: %v", err)
		}
		configPath = filepath.Join(directory, "gateway.sqlite")
	}
	absoluteDB, err := filepath.Abs(configPath)
	if err != nil {
		diagnostics.Fatal("HGW-MAIN-003", "No se puede continuar: %v", err)
	}
	configPath = absoluteDB
	if opts.ApplyStagedRestore != "" {
		if err := applyStagedRestore(configPath, opts.ApplyStagedRestore); err != nil {
			diagnostics.Fatal("HGW-RESTORE-ERROR", "%v", err)
		}
		if err := restartAfterStagedRestore(); err != nil {
			diagnostics.Fatal("HGW-MAIN-005", "No se puede continuar: %v", err)
		}
		return
	}
	if logFile == "" {
		logFile = filepath.Join(filepath.Dir(configPath), "logs", "gateway.log")
	}
	closeLog, logErr := diagnostics.Setup(logFile, opts.LogMaxMB, opts.LogBackups)
	if logErr != nil {
		diagnostics.Fatal("HGW-LOG-001", "no se pudo abrir el archivo de log: %v", logErr)
	}
	defer closeLog()
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-START-001", "registro inicializado; archivo=%s", logFile)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOG-LEVEL", "nivel=%s; modo=%s", diagnostics.EffectiveLevel().Label(), diagnostics.EffectiveLevel().Label())
	if opts.Reload {
		if err := requestReload(configPath); err != nil {
			diagnostics.Fatal("HGW-MAIN-008", "No se puede continuar: %v", err)
		}
		fmt.Println(i18n.T("Solicitud de recarga enviada; consulte los logs del Gateway para confirmar el resultado"))
		return
	}
	if opts.RestoreBackup != "" {
		saved, e := state.RestoreOffline(configPath, opts.RestoreBackup)
		if e != nil {
			diagnostics.Fatal("HGW-RESTORE-ERR", "restauración cancelada: %v", e)
		}
		fmt.Println(i18n.T("Restauración local completada. Estado anterior preservado en:"), saved)
		fmt.Println(i18n.T("Las ACL remotas de Headscale NO se han restaurado. Reinicia Gateway."))
		return
	}
	// Primer arranque: inicializar las tablas y valores seguros de SQLite.
	if err := ensureSetupDatabase(configPath); err != nil {
		diagnostics.Fatal("HGW-MAIN-010", "No se puede continuar: %v", err)
	}
	restoreLogSettings(configPath, cliLevel != "" || envLevel != "")

	if opts.ResetRecovery {
		db, err := openCatalog(configPath)
		if err != nil {
			diagnostics.Fatal("HGW-MAIN-011", "No se puede continuar: %v", err)
		}
		if err = rotateRecoveryCredential(db); err != nil {
			_ = db.Close()
			diagnostics.Fatal("HGW-MAIN-012", "No se puede continuar: %v", err)
		}
		_ = db.Close()
		fmt.Println(i18n.T("Clave rotada en SQLite; reinicia Gateway para activar el código temporal de primer acceso"))
		return
	}
	if opts.SetDomain != "" || opts.SetHeadscale != "" {
		db, err := openCatalog(configPath)
		if err != nil {
			diagnostics.Fatal("HGW-MAIN-013", "No se puede continuar: %v", err)
		}
		values, err := db.GetSettings()
		if err != nil {
			_ = db.Close()
			diagnostics.Fatal("HGW-MAIN-014", "No se puede continuar: %v", err)
		}
		if opts.SetDomain != "" {
			values["domain.base_domain"] = opts.SetDomain
		}
		if opts.SetHeadscale != "" {
			values["headscale.url"] = opts.SetHeadscale
		}
		err = db.ReplaceSettings(values)
		_ = db.Close()
		if err != nil {
			diagnostics.Fatal("HGW-MAIN-015", "No se puede continuar: %v", err)
		}
	}
	ctx, stop := signal.NotifyContext(processContext, os.Interrupt, syscall.SIGTERM)
	defer stop()
	processContext = ctx
	startGateway(configPath)
	shutdownTasks.Wait()
}
