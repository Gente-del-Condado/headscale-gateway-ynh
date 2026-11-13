//go:build !legacy_sqlite

package state

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// BackupTo snapshots all committed rows (including WAL) using SQLite's
// native VACUUM INTO command without copying a live SQLite file.
func (s *Store) BackupTo(dir string) (err error) {
	if err = os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("crear directorio privado de backup: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	target := filepath.Join(dir, "gateway.sqlite")
	s.mu.Lock()
	_, err = s.db.Exec("VACUUM INTO ?", target)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("copia consistente SQLite: %w", err)
	}
	if err = os.Chmod(target, 0600); err != nil {
		return err
	}
	key, err := os.ReadFile(filepath.Join(filepath.Dir(s.path), "master.key"))
	if err != nil {
		return err
	}
	if len(key) != 32 {
		return errors.New("clave AES incompatible")
	}
	file, err := os.OpenFile(filepath.Join(dir, "master.key"), os.O_WRONLY|os.O_EXCL|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(key); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
func (s *Store) BackupRoot() string { return filepath.Join(filepath.Dir(s.path), "backups") }

func checkBackupSQLite(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	header := make([]byte, 100)
	if _, e = io.ReadFull(f, header); e != nil {
		return errors.New("la copia SQLite está incompleta")
	}
	if !bytes.HasPrefix(header, []byte("SQLite format 3\x00")) {
		return errors.New("la copia no contiene una base SQLite válida")
	}
	// Abre en solo lectura: una entrada dañada no debe crear una base nueva.
	cleanPath := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		cleanPath = "/" + cleanPath
	}
	readOnly := url.URL{Scheme: "file", Path: cleanPath, RawQuery: "mode=ro"}
	db, e := sql.Open("sqlite", readOnly.String())
	if e != nil {
		return e
	}
	defer db.Close()
	var result string
	if e = db.QueryRow("PRAGMA quick_check").Scan(&result); e != nil {
		return fmt.Errorf("integridad SQLite: %w", e)
	}
	if result != "ok" {
		return errors.New("la copia SQLite no supera PRAGMA quick_check")
	}
	return nil
}
func ValidateBackupPair(dbPath, keyPath string) error {
	if e := checkBackupSQLite(dbPath); e != nil {
		return e
	}
	key, e := os.ReadFile(keyPath)
	if e != nil {
		return e
	}
	if len(key) != 32 {
		return errors.New("clave maestra de copia incorrecta")
	}
	return nil
}
func privateCopy(src, dst string) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = io.Copy(out, in); e != nil {
		out.Close()
		return e
	}
	if e = out.Sync(); e != nil {
		out.Close()
		return e
	}
	return out.Close()
}

// RestoreOffline exige el Gateway detenido. El llamante debe cerrar SQLite
// y tsnet antes de invocarlo. Los archivos anteriores se conservan.
func RestoreOffline(activeDB, backupDir string) (string, error) {
	activeDB, e := filepath.Abs(activeDB)
	if e != nil {
		return "", e
	}
	backupDir, e = filepath.Abs(backupDir)
	if e != nil {
		return "", e
	}
	if activeDB == filepath.Join(backupDir, "gateway.sqlite") {
		return "", errors.New("origen y destino coinciden")
	}
	info, e := os.Lstat(backupDir)
	if e != nil {
		return "", e
	}
	if !privateDirectory(info) {
		return "", errors.New("directorio de origen inseguro")
	}
	for _, name := range []string{"gateway.sqlite", "master.key"} {
		info, e = os.Lstat(filepath.Join(backupDir, name))
		if e != nil {
			return "", e
		}
		if !privateFile(info) {
			return "", fmt.Errorf("archivo %s inseguro", name)
		}
	}
	if e = ValidateBackupPair(filepath.Join(backupDir, "gateway.sqlite"), filepath.Join(backupDir, "master.key")); e != nil {
		return "", e
	}
	dest := filepath.Dir(activeDB)
	if e = os.MkdirAll(dest, 0700); e != nil {
		return "", e
	}
	staging, e := os.MkdirTemp(dest, ".restore-stage-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(staging)
	for _, name := range []string{"gateway.sqlite", "master.key"} {
		if e = privateCopy(filepath.Join(backupDir, name), filepath.Join(staging, name)); e != nil {
			return "", e
		}
	}
	oldDir := filepath.Join(dest, "before-restore-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if e = os.Mkdir(oldDir, 0700); e != nil {
		return "", e
	}
	originals := []string{activeDB, filepath.Join(dest, "master.key"), activeDB + "-wal", activeDB + "-shm"}
	moved := []string{}
	rollback := func() {
		_ = os.Remove(activeDB)
		_ = os.Remove(filepath.Join(dest, "master.key"))
		for i := len(moved) - 1; i >= 0; i-- {
			path := moved[i]
			_ = os.Rename(filepath.Join(oldDir, filepath.Base(path)), path)
		}
	}
	for _, path := range originals {
		info, e = os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !info.Mode().IsRegular() {
			rollback()
			return "", errors.New("archivo activo no regular o inaccesible")
		}
		if e = os.Rename(path, filepath.Join(oldDir, filepath.Base(path))); e != nil {
			rollback()
			return "", e
		}
		moved = append(moved, path)
	}
	if e = os.Rename(filepath.Join(staging, "gateway.sqlite"), activeDB); e != nil {
		rollback()
		return "", e
	}
	if e = os.Rename(filepath.Join(staging, "master.key"), filepath.Join(dest, "master.key")); e != nil {
		rollback()
		return "", e
	}
	return oldDir, nil
}
