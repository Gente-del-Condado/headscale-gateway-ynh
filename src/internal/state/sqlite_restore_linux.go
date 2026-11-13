//go:build legacy_sqlite && linux && cgo

package state

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

func checkBackupSQLite(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw := make([]byte, 100)
	if _, err = io.ReadFull(f, raw); err != nil {
		return errors.New("la copia SQLite está incompleta")
	}
	if !bytes.HasPrefix(raw, []byte("SQLite format 3\x00")) {
		return errors.New("la copia no contiene una base SQLite válida")
	}
	// Abre en solo lectura y comprueba la integridad antes de mover archivos activos.
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var db *C.sqlite3
	if rc := C.sqlite3_open_v2(cpath, &db, C.SQLITE_OPEN_READONLY|C.SQLITE_OPEN_FULLMUTEX, nil); rc != C.SQLITE_OK {
		if db != nil {
			C.sqlite3_close(db)
		}
		return errors.New("SQLite no permite abrir la copia")
	}
	defer C.sqlite3_close(db)
	query := C.CString("PRAGMA quick_check")
	defer C.free(unsafe.Pointer(query))
	var stmt *C.sqlite3_stmt
	if C.sqlite3_prepare_v2(db, query, -1, &stmt, nil) != C.SQLITE_OK {
		return errors.New("no se pudo comprobar integridad SQLite")
	}
	defer C.sqlite3_finalize(stmt)
	if C.sqlite3_step(stmt) != C.SQLITE_ROW || C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))) != "ok" {
		return errors.New("la copia SQLite no supera PRAGMA quick_check")
	}
	return nil
}

func privateCopy(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	file, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(file, input); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// RestoreOffline solo se invoca con el Gateway detenido. Valida siempre la copia
// y conserva la base y la clave existentes para poder revertir.
// No vuelve a aplicar instantáneas de políticas al servidor Headscale remoto.
func RestoreOffline(activeDB, backupDir string) (string, error) {
	activeDB, err := filepath.Abs(activeDB)
	if err != nil {
		return "", err
	}
	backupDir, err = filepath.Abs(backupDir)
	if err != nil {
		return "", err
	}
	if activeDB == filepath.Join(backupDir, "gateway.sqlite") {
		return "", errors.New("origen y destino coinciden")
	}
	info, err := os.Lstat(backupDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("directorio origen debe ser privado (0700)")
	}
	for _, name := range []string{"gateway.sqlite", "master.key"} {
		fi, e := os.Lstat(filepath.Join(backupDir, name))
		if e != nil {
			return "", e
		}
		if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
			return "", fmt.Errorf("archivo %s inseguro", name)
		}
	}
	key, err := os.ReadFile(filepath.Join(backupDir, "master.key"))
	if err != nil {
		return "", err
	}
	if len(key) != 32 {
		return "", errors.New("master.key inválida")
	}
	if err = checkBackupSQLite(filepath.Join(backupDir, "gateway.sqlite")); err != nil {
		return "", err
	}
	dest := filepath.Dir(activeDB)
	if err = os.MkdirAll(dest, 0700); err != nil {
		return "", err
	}
	// Stage before touching current files. No symlinks or outside paths.
	staging, err := os.MkdirTemp(dest, ".restore-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err = privateCopy(filepath.Join(backupDir, "gateway.sqlite"), filepath.Join(staging, "gateway.sqlite")); err != nil {
		return "", err
	}
	if err = privateCopy(filepath.Join(backupDir, "master.key"), filepath.Join(staging, "master.key")); err != nil {
		return "", err
	}
	oldDir := filepath.Join(dest, "before-restore-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err = os.Mkdir(oldDir, 0700); err != nil {
		return "", err
	}
	// Mueve los archivos anteriores, incluidos WAL/SHM, antes de instalar el nuevo par.
	originals := []string{activeDB, filepath.Join(dest, "master.key"), activeDB + "-wal", activeDB + "-shm"}
	moved := []string{}
	rollback := func() {
		_ = os.Remove(activeDB)
		_ = os.Remove(filepath.Join(dest, "master.key"))
		for i := len(moved) - 1; i >= 0; i-- {
			name := moved[i]
			_ = os.Rename(filepath.Join(oldDir, filepath.Base(name)), name)
		}
	}
	for _, path := range originals {
		fi, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !fi.Mode().IsRegular() {
			rollback()
			return "", errors.New("archivo activo no regular o inaccesible")
		}
		if err = os.Rename(path, filepath.Join(oldDir, filepath.Base(path))); err != nil {
			rollback()
			return "", err
		}
		moved = append(moved, path)
	}
	if err = os.Rename(filepath.Join(staging, "gateway.sqlite"), activeDB); err != nil {
		rollback()
		return "", err
	}
	if err = os.Rename(filepath.Join(staging, "master.key"), filepath.Join(dest, "master.key")); err != nil {
		rollback()
		return "", err
	}
	return oldDir, nil
}
