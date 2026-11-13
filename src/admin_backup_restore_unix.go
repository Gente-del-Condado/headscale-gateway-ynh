//go:build linux || darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

var restoreScheduled atomic.Bool

// El estado reside junto a data_dir: la restauración sustituye ese directorio
// y debe conservar el único registro de su resultado.
func writeRestoreStatus(dbPath, job, phase, detail string) {
	path := filepath.Join(filepath.Dir(filepath.Dir(dbPath)), ".hgw-restore-status.json")
	value := struct {
		Job     string `json:"job"`
		Phase   string `json:"phase"`
		Detail  string `json:"detail"`
		Updated string `json:"updated_at"`
	}{job, phase, detail, time.Now().UTC().Format(time.RFC3339)}
	raw, e := json.Marshal(value)
	if e != nil {
		return
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".restore-state-*")
	if e != nil {
		return
	}
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(raw); e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if e = os.Rename(tmp.Name(), path); e != nil {
		_ = os.Remove(tmp.Name())
	}
}

// ScheduleBackupRestore valida y prepara todo antes de sustituir el proceso.
// La restauración no escribe ninguna ACL remota.
func (a *adminController) ScheduleBackupRestore(ctx context.Context, archive, password string) (string, error) {
	if !restoreScheduled.CompareAndSwap(false, true) {
		return "", errors.New("restauración en curso")
	}
	good := false
	defer func() {
		if !good {
			restoreScheduled.Store(false)
		}
	}()
	dbPath, e := filepath.Abs(a.configPath)
	if e != nil {
		return "", e
	}
	dataDir := filepath.Dir(dbPath)
	stage, e := os.MkdirTemp(filepath.Dir(dataDir), ".hgw-restore-")
	if e != nil {
		return "", e
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(stage)
		}
	}()
	manifest, e := securebackup.Extract(ctx, archive, password, stage)
	if e != nil {
		return "", fmt.Errorf("copia no verificable: %w", e)
	}
	if manifest.Mode != "all" && manifest.Mode != "selected" {
		return "", errors.New("tipo de copia incompatible")
	}
	if manifest.Mode == "all" {
		if _, e = os.Stat(filepath.Join(stage, "data", "gateway.sqlite")); e != nil {
			return "", errors.New("copia total sin SQLite")
		}
	}
	if e = os.WriteFile(filepath.Join(stage, ".validated"), []byte(dbPath), 0600); e != nil {
		return "", e
	}
	if e = os.WriteFile(filepath.Join(stage, ".mode"), []byte(manifest.Mode), 0600); e != nil {
		return "", e
	}
	// Main process remains unchanged until syscall.Exec transitions to offline mode.
	writeRestoreStatus(dbPath, filepath.Base(stage), "verified", "Archivo descifrado y verificado; SQLite cerrará antes de restaurar")
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-RESTORE-READY", "verificación criptográfica y SHA-256 correcta; iniciando modo offline")
	good = true
	done = true
	go func() {
		time.Sleep(2 * time.Second)
		executable, err := os.Executable()
		if err == nil {
			args := append(append([]string{executable}, os.Args[1:]...), "--apply-staged-restore", stage)
			err = syscall.Exec(executable, args, os.Environ())
		}
		if err != nil {
			writeRestoreStatus(dbPath, filepath.Base(stage), "failed", "No se pudo entrar en modo de restauración")
			diagnostics.Emit("admin", diagnostics.Error, "HGW-RESTORE-ERROR", "reexec: %v", err)
			restoreScheduled.Store(false)
		}
	}()
	return filepath.Base(stage), nil
}

// applyStagedRestore solo se invoca desde el ejecutable de reemplazo, con SQLite
// y tsnet detenidos. El estado anterior se conserva en disco.
func applyStagedRestore(activeDB, stage string) (retErr error) {
	abs, e := filepath.Abs(stage)
	if e != nil {
		return e
	}
	stage = abs
	dataDir := filepath.Dir(activeDB)
	job := filepath.Base(stage)
	if stage == dataDir || filepath.Dir(stage) != filepath.Dir(dataDir) || !strings.HasPrefix(job, ".hgw-restore-") {
		return errors.New("carpeta temporal de restauración inesperada")
	}
	info, e := os.Lstat(stage)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return errors.New("carpeta temporal insegura")
	}
	markerPath := filepath.Join(stage, ".validated")
	markerInfo, e := os.Lstat(markerPath)
	if e != nil {
		return e
	}
	if !markerInfo.Mode().IsRegular() || markerInfo.Mode().Perm()&0077 != 0 || markerInfo.Size() != int64(len(activeDB)) {
		return errors.New("restauración no corresponde a esta instalación")
	}
	expected, e := os.ReadFile(markerPath)
	if e != nil {
		return e
	}
	if string(expected) != activeDB {
		return errors.New("restauración no corresponde a esta instalación")
	}
	// Solo el directorio privado preparado para esta SQLite se puede limpiar.
	defer func() {
		if retErr != nil {
			writeRestoreStatus(activeDB, job, "failed", retErr.Error())
			_ = os.RemoveAll(stage)
		}
	}()
	writeRestoreStatus(activeDB, job, "applying", "Gateway detenido; sustituyendo datos y comprobando SQLite")
	restored := filepath.Join(stage, "data")
	info, e = os.Lstat(restored)
	if e != nil {
		return e
	}
	if !info.IsDir() {
		return errors.New("datos restaurados inválidos")
	}
	// Inspect files before touching current state.
	var files []string
	walkErr := filepath.WalkDir(restored, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			rel, er := filepath.Rel(restored, p)
			if er != nil {
				return er
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	hasDB, hasKey := false, false
	for _, name := range files {
		if name == "gateway.sqlite" {
			hasDB = true
		}
		if name == "master.key" {
			hasKey = true
		}
	}
	if hasDB != hasKey {
		return errors.New("SQLite y clave maestra deben restaurarse juntas")
	}
	if hasDB {
		if err := state.ValidateBackupPair(filepath.Join(restored, "gateway.sqlite"), filepath.Join(restored, "master.key")); err != nil {
			return fmt.Errorf("integridad SQLite/restauración: %w", err)
		}
	}
	full := false
	// El manifiesto que escribe el controlador administrativo identifica la preparación.
	marker, er := os.ReadFile(filepath.Join(stage, ".mode"))
	if er == nil && string(marker) == "all" {
		full = true
	}
	if full {
		if !hasDB {
			return errors.New("copia completa sin SQLite")
		}
		old := dataDir + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if e = os.Rename(dataDir, old); e != nil {
			return e
		}
		if e = os.Rename(restored, dataDir); e != nil {
			_ = os.Rename(old, dataDir)
			return e
		}
		// Conserva los archivos locales excluidos intencionadamente de la exportación.
		if _, e = os.Stat(filepath.Join(old, "backups")); e == nil {
			_ = os.Rename(filepath.Join(old, "backups"), filepath.Join(dataDir, "backups"))
		}
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-RESTORE-FULL-DONE", "Restauración integral correcta; estado anterior en %s", old)
	} else {
		if hasDB {
			if _, e = state.RestoreOffline(activeDB, restored); e != nil {
				return e
			}
		}
		// Aplica la identidad después de que exec cierre tsnet; revierte directorios si falla.
		rollbackDir := dataDir + ".identity-before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		doneDirs := map[string]bool{}
		for _, name := range files {
			top := strings.Split(name, "/")[0]
			if doneDirs[top] {
				continue
			}
			doneDirs[top] = true
			if top != "identity" && !strings.HasPrefix(top, "tailscale-state-") && !strings.HasPrefix(top, "tailscale-") {
				continue
			}
			src := filepath.Join(restored, top)
			dst := filepath.Join(dataDir, top)
			if _, e = os.Stat(src); e != nil {
				continue
			}
			if e = os.MkdirAll(rollbackDir, 0700); e != nil {
				return e
			}
			if _, e = os.Stat(dst); e == nil {
				if e = os.Rename(dst, filepath.Join(rollbackDir, top)); e != nil {
					return e
				}
			}
			if e = os.Rename(src, dst); e != nil {
				return e
			}
		}
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-RESTORE-SELECTIVE-DONE", "Restauración selectiva completada")
	}
	if e = os.RemoveAll(stage); e != nil {
		return e
	}
	writeRestoreStatus(activeDB, job, "completed", "Restauración local completada; el servidor se está reiniciando")
	return nil
}

// Reinicia el mismo ejecutable bajo el supervisor y los argumentos existentes.
func restartAfterStagedRestore() error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	args := []string{executable}
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--apply-staged-restore" {
			i++
			continue
		}
		args = append(args, os.Args[i])
	}
	return syscall.Exec(executable, args, os.Environ())
}
