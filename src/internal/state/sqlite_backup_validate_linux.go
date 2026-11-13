//go:build legacy_sqlite && linux && cgo

package state

import (
	"errors"
	"os"
)

// ValidateBackupPair comprueba SQLite restaurado y su clave maestra antes
// de sustituir el directorio de datos activo del Gateway.
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
