//go:build legacy_sqlite && linux && cgo

package state

/*
#include <sqlite3.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

// BackupTo crea atómicamente un directorio privado con una instantánea coherente
// de SQLite y su clave maestra AES. Se invoca con el proceso en marcha.
// El estado de Tailscale y CertMagic pertenece a bibliotecas externas y no se
// incluye aquí; para copiarlo también, usa una instantánea del sistema de archivos.
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
	err = s.snapshot(target)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	key, err := os.ReadFile(filepath.Join(filepath.Dir(s.path), "master.key"))
	if err != nil {
		return err
	}
	if len(key) != 32 {
		return fmt.Errorf("clave AES incompatible")
	}
	file, err := os.OpenFile(filepath.Join(dir, "master.key"), os.O_WRONLY|os.O_EXCL|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(key); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}
func (s *Store) snapshot(path string) error {
	// El destino no existe porque su directorio acaba de crearse. Lo abre para
	// lectura/escritura y creación; utiliza la API de copias SQLite, nunca cp.
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var target *C.sqlite3
	if rc := C.sqlite3_open_v2(cpath, &target, C.SQLITE_OPEN_READWRITE|C.SQLITE_OPEN_CREATE|C.SQLITE_OPEN_FULLMUTEX, nil); rc != C.SQLITE_OK {
		if target != nil {
			C.sqlite3_close(target)
		}
		return fmt.Errorf("abrir destino SQLite: %d", int(rc))
	}
	defer C.sqlite3_close(target)
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	main := C.CString("main")
	defer C.free(unsafe.Pointer(main))
	backup := C.sqlite3_backup_init(target, main, s.db, main)
	if backup == nil {
		return fmt.Errorf("SQLite backup_init: %d", int(C.sqlite3_errcode(target)))
	}
	rc := C.sqlite3_backup_step(backup, -1)
	finish := C.sqlite3_backup_finish(backup)
	if rc != C.SQLITE_DONE || finish != C.SQLITE_OK {
		return fmt.Errorf("SQLite backup falló: step=%d finish=%d", int(rc), int(finish))
	}
	return nil
}

// BackupRoot se deriva del directorio SQLite, nunca de un parámetro HTTP.
// Los archivos de este directorio contienen la clave maestra AES.
func (s *Store) BackupRoot() string { return filepath.Join(filepath.Dir(s.path), "backups") }
