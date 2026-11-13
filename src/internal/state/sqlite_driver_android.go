//go:build android && !legacy_sqlite

package state

import (
	"database/sql"

	"github.com/mattn/go-sqlite3"
)

// Bionic utiliza las llamadas de sistema admitidas por seccomp en Android.
// El esquema, los PRAGMA y el formato de copias siguen siendo los comunes.
func init() { sql.Register("sqlite", &sqlite3.SQLiteDriver{}) }
