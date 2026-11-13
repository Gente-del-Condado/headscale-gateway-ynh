//go:build legacy_sqlite && linux && cgo

// El paquete state guarda política administrativa y auditoría en SQLite.
// Enlaza SQLite mediante el paquete de desarrollo sqlite3 del sistema, sin
// necesitar comandos de shell ni un servicio externo de base de datos.
package state

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"
)

type Store struct {
	mu     sync.Mutex
	db     *C.sqlite3
	sealer *secretBox
	path   string
}
type AuditEvent struct {
	Time    string `json:"time"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

// Open crea SQLite con permisos 0600, registro WAL y versionado del esquema.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("SQLite debe ser un archivo regular con permisos 0600")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file := C.CString(path)
	defer C.free(unsafe.Pointer(file))
	var db *C.sqlite3
	if rc := C.sqlite3_open_v2(file, &db, C.SQLITE_OPEN_READWRITE|C.SQLITE_OPEN_CREATE|C.SQLITE_OPEN_FULLMUTEX, nil); rc != C.SQLITE_OK {
		if db != nil {
			C.sqlite3_close(db)
		}
		return nil, fmt.Errorf("abrir SQLite: %d", int(rc))
	}
	C.sqlite3_busy_timeout(db, 5000)
	keyPath := filepath.Join(filepath.Dir(path), "master.key")
	// Nunca regenera la clave maestra de una base existente: impediría recuperar
	// permanentemente sus ajustes y credenciales cifrados.
	if _, keyErr := os.Stat(keyPath); os.IsNotExist(keyErr) {
		var count int64
		if fi, statErr := os.Stat(path); statErr == nil {
			count = fi.Size()
		}
		// A newly created SQLite file can still contain a nonempty header.
		// Check for an existing encrypted catalog instead of guessing by size.
		if count > 0 {
			probe := C.CString("SELECT count(*) FROM sqlite_master WHERE name IN ('metadata','gateway_settings','gateway_services')")
			var stmt *C.sqlite3_stmt
			if C.sqlite3_prepare_v2(db, probe, -1, &stmt, nil) == C.SQLITE_OK {
				if C.sqlite3_step(stmt) == C.SQLITE_ROW && C.sqlite3_column_int(stmt, 0) > 0 {
					C.sqlite3_finalize(stmt)
					C.free(unsafe.Pointer(probe))
					_ = C.sqlite3_close(db)
					return nil, errors.New("master.key no existe para esta base de datos; restaura la copia de seguridad")
				}
				C.sqlite3_finalize(stmt)
			}
			C.free(unsafe.Pointer(probe))
		}
	} else if keyErr != nil {
		_ = C.sqlite3_close(db)
		return nil, keyErr
	}
	sealer, err := openSecretBox(keyPath)
	if err != nil {
		_ = C.sqlite3_close(db)
		return nil, fmt.Errorf("clave maestra: %w", err)
	}
	s := &Store{db: db, sealer: sealer, path: path}
	if err := os.Chmod(path, 0600); err != nil {
		s.Close()
		return nil, err
	}
	for _, sql := range []string{
		"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON",
		"CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS audit (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS api_tokens (hash TEXT PRIMARY KEY, label TEXT NOT NULL, role TEXT NOT NULL, scopes TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL)",
	} {
		if err := s.exec(sql); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) err() error { return errors.New(C.GoString(C.sqlite3_errmsg(s.db))) }
func (s *Store) exec(q string) error {
	c := C.CString(q)
	defer C.free(unsafe.Pointer(c))
	var msg *C.char
	rc := C.sqlite3_exec(s.db, c, nil, nil, &msg)
	if rc != C.SQLITE_OK {
		if msg != nil {
			defer C.sqlite3_free(unsafe.Pointer(msg))
			return errors.New(C.GoString(msg))
		}
		return s.err()
	}
	return nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	rc := C.sqlite3_close(s.db)
	if rc == C.SQLITE_OK {
		s.db = nil
		return nil
	}
	return fmt.Errorf("cerrar SQLite: %d", int(rc))
}
func (s *Store) prepare(q string) (*C.sqlite3_stmt, error) {
	c := C.CString(q)
	defer C.free(unsafe.Pointer(c))
	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(s.db, c, -1, &stmt, nil); rc != C.SQLITE_OK {
		return nil, s.err()
	}
	return stmt, nil
}
func bind(stmt *C.sqlite3_stmt, pos int, v string) error {
	c := C.CString(v)
	defer C.free(unsafe.Pointer(c))
	if C.sqlite3_bind_text(stmt, C.int(pos), c, C.int(len(v)), C.SQLITE_TRANSIENT) != C.SQLITE_OK {
		return errors.New("falló bind SQLite")
	}
	return nil
}
func (s *Store) PutJSON(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sealed, err := s.sealer.seal(string(b))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value")
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)
	if err = bind(stmt, 1, key); err != nil {
		return err
	}
	if err = bind(stmt, 2, sealed); err != nil {
		return err
	}
	if C.sqlite3_step(stmt) != C.SQLITE_DONE {
		return s.err()
	}
	return nil
}
func (s *Store) GetJSON(key string, v any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("SELECT value FROM metadata WHERE key=?")
	if err != nil {
		return false, err
	}
	defer C.sqlite3_finalize(stmt)
	if err = bind(stmt, 1, key); err != nil {
		return false, err
	}
	switch C.sqlite3_step(stmt) {
	case C.SQLITE_ROW:
		encoded := C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0))))
		plain, e := s.sealer.open(encoded)
		if e != nil {
			return false, e
		}
		return true, json.Unmarshal([]byte(plain), v)
	case C.SQLITE_DONE:
		return false, nil
	default:
		return false, s.err()
	}
}

// Audit records administrative actions, not request bodies, tokens or passwords.
func (s *Store) Audit(actor, action, outcome string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("INSERT INTO audit(happened_at,actor,action,outcome) VALUES(?,?,?,?)")
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)
	for i, v := range []string{time.Now().UTC().Format(time.RFC3339), actor, action, outcome} {
		if err = bind(stmt, i+1, v); err != nil {
			return err
		}
	}
	if C.sqlite3_step(stmt) != C.SQLITE_DONE {
		return s.err()
	}
	return nil
}
func (s *Store) ListAudit(limit int) ([]AuditEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.prepare("SELECT happened_at,actor,action,outcome FROM audit ORDER BY id DESC LIMIT ?")
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)
	if C.sqlite3_bind_int(stmt, 1, C.int(limit)) != C.SQLITE_OK {
		return nil, s.err()
	}
	result := []AuditEvent{}
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			return result, nil
		}
		if rc != C.SQLITE_ROW {
			return nil, s.err()
		}
		get := func(i int) string {
			return C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, C.int(i)))))
		}
		result = append(result, AuditEvent{get(0), get(1), get(2), get(3)})
	}
}

// PageAudit uses a strict column whitelist; parameters never become SQL identifiers.
func (s *Store) PageAudit(page, size int, sortBy, order string) ([]AuditEvent, int, error) {
	if page < 1 || page > 100000 || size < 1 || size > 50 {
		return nil, 0, fmt.Errorf("paginación inválida")
	}
	columns := map[string]string{"time": "happened_at", "actor": "actor", "action": "action", "outcome": "outcome"}
	col := columns[sortBy]
	if col == "" {
		col = "id"
	}
	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	countStmt, err := s.prepare("SELECT COUNT(*) FROM audit")
	if err != nil {
		return nil, 0, err
	}
	total := 0
	if C.sqlite3_step(countStmt) == C.SQLITE_ROW {
		total = int(C.sqlite3_column_int(countStmt, 0))
	}
	C.sqlite3_finalize(countStmt)
	stmt, err := s.prepare("SELECT happened_at,actor,action,outcome FROM audit ORDER BY " + col + " " + direction + ",id DESC LIMIT ? OFFSET ?")
	if err != nil {
		return nil, 0, err
	}
	defer C.sqlite3_finalize(stmt)
	if C.sqlite3_bind_int(stmt, 1, C.int(size)) != C.SQLITE_OK || C.sqlite3_bind_int(stmt, 2, C.int((page-1)*size)) != C.SQLITE_OK {
		return nil, 0, s.err()
	}
	events := []AuditEvent{}
	for {
		result := C.sqlite3_step(stmt)
		if result == C.SQLITE_DONE {
			return events, total, nil
		}
		if result != C.SQLITE_ROW {
			return nil, 0, s.err()
		}
		get := func(i int) string {
			return C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, C.int(i)))))
		}
		events = append(events, AuditEvent{get(0), get(1), get(2), get(3)})
	}
}
