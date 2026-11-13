//go:build legacy_sqlite && linux && cgo

package state

/*
#include <sqlite3.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// ServiceRecord y TemplateRecord son las estructuras persistidas del catálogo.
// No dependen de la presentación HTTP ni de los modelos de ejecución del proxy.
type ServiceRecord struct {
	Description string          `json:"description,omitempty"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	Backend     string          `json:"backend"`
	Template    string          `json:"template,omitempty"`
	Proxy       json.RawMessage `json:"proxy,omitempty"`
}
type TemplateRecord struct {
	Name        string          `json:"name"`
	Category    string          `json:"category,omitempty"`
	Description string          `json:"description,omitempty"`
	BackendHint string          `json:"backend_hint,omitempty"`
	Proxy       json.RawMessage `json:"proxy"`
}

// InitCatalog aplica migraciones aditivas y transaccionales. Conserva las tablas
// de credenciales y tokens API existentes, sin escribir archivos YAML.
func (s *Store) InitCatalog() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.queryRows("PRAGMA user_version", 1)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		version, e := strconv.Atoi(rows[0][0])
		if e != nil {
			return e
		}
		if version >= 3 {
			return nil
		}
	}

	if err := s.exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.exec("ROLLBACK")
		}
	}()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS gateway_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS gateway_services (name TEXT PRIMARY KEY, json TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS gateway_templates (name TEXT PRIMARY KEY, json TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_gateway_services_updated ON gateway_services(updated_at)`,
		`PRAGMA user_version=3`,
	} {
		if err := s.exec(stmt); err != nil {
			return err
		}
	}
	if err := s.exec("COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

// queryRows ejecuta consultas preparadas de solo lectura, nunca interpola SQL.
func (s *Store) queryRows(query string, columns int) ([][]string, error) {
	stmt, err := s.prepare(query)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)
	out := [][]string{}
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			return out, nil
		}
		if rc != C.SQLITE_ROW {
			return nil, s.err()
		}
		row := make([]string, columns)
		for i := 0; i < columns; i++ {
			value := C.sqlite3_column_text(stmt, C.int(i))
			if value != nil {
				row[i] = C.GoString((*C.char)(unsafe.Pointer(value)))
			}
		}
		out = append(out, row)
	}
}
func (s *Store) executeBind(query string, values ...string) error {
	stmt, err := s.prepare(query)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)
	for i, v := range values {
		if err = bind(stmt, i+1, v); err != nil {
			return err
		}
	}
	if C.sqlite3_step(stmt) != C.SQLITE_DONE {
		return s.err()
	}
	return nil
}
func (s *Store) GetSettings() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.queryRows("SELECT key,value FROM gateway_settings ORDER BY key", 2)
	if err != nil {
		return nil, err
	}
	settings := make(map[string]string, len(rows))
	for _, row := range rows {
		plain, e := s.sealer.open(row[1])
		if e != nil {
			return nil, fmt.Errorf("ajuste %s: %w", row[0], e)
		}
		settings[row[0]] = plain
	}
	return settings, nil
}

// ReplaceSettings sustituye toda la configuración de forma atómica tras validar
// la instantánea candidata; nunca actualiza los valores de forma incremental.
func (s *Store) ReplaceSettings(settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.exec("ROLLBACK")
		}
	}()
	if err := s.exec("DELETE FROM gateway_settings"); err != nil {
		return err
	}
	for k, v := range settings {
		if k == "" || len(k) > 128 || len(v) > 16384 {
			return fmt.Errorf("clave o valor de configuración inválido: %q", k)
		}
		ciphertext, e := s.sealer.seal(v)
		if e != nil {
			return e
		}
		if err := s.executeBind("INSERT INTO gateway_settings(key,value) VALUES(?,?)", k, ciphertext); err != nil {
			return err
		}
	}
	if err := s.exec("COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}
func (s *Store) ListServices() ([]ServiceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.queryRows("SELECT json FROM gateway_services ORDER BY name", 1)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceRecord, 0, len(rows))
	for _, r := range rows {
		plain, e := s.sealer.open(r[0])
		if e != nil {
			return nil, e
		}
		var val ServiceRecord
		if e := json.Unmarshal([]byte(plain), &val); e != nil {
			return nil, e
		}
		out = append(out, val)
	}
	return out, nil
}
func (s *Store) SaveService(item ServiceRecord) error {
	if item.Name == "" || len(item.Name) > 63 || len(item.Backend) > 8192 || len(item.Proxy) > 32768 || len(item.Description) > 1024 {
		return fmt.Errorf("tamaño de servicio inválido")
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	sealed, err := s.sealer.seal(string(raw))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeBind(`INSERT INTO gateway_services(name,json,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
        ON CONFLICT(name) DO UPDATE SET json=excluded.json,updated_at=CURRENT_TIMESTAMP`, item.Name, sealed)
}
func (s *Store) DeleteService(name string) error {
	if name == "" {
		return fmt.Errorf("servicio vacío")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeBind("DELETE FROM gateway_services WHERE name=?", name)
}
func (s *Store) ListTemplates() ([]TemplateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.queryRows("SELECT json FROM gateway_templates ORDER BY name", 1)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateRecord, 0, len(rows))
	for _, r := range rows {
		plain, e := s.sealer.open(r[0])
		if e != nil {
			return nil, e
		}
		var val TemplateRecord
		if e := json.Unmarshal([]byte(plain), &val); e != nil {
			return nil, e
		}
		out = append(out, val)
	}
	return out, nil
}
func (s *Store) SaveTemplate(item TemplateRecord) error {
	if item.Name == "" || len(item.Name) > 63 || len(item.Proxy) > 32768 || len(item.Description) > 1024 || len(item.Category) > 80 || len(item.BackendHint) > 256 || !json.Valid(item.Proxy) {
		return fmt.Errorf("plantilla inválida")
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	sealed, err := s.sealer.seal(string(raw))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeBind(`INSERT INTO gateway_templates(name,json,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
        ON CONFLICT(name) DO UPDATE SET json=excluded.json,updated_at=CURRENT_TIMESTAMP`, item.Name, sealed)
}
func (s *Store) DeleteTemplate(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("plantilla inválida")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeBind("DELETE FROM gateway_templates WHERE name=?", name)
}
