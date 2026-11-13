//go:build !legacy_sqlite

// Package state conserva el mismo catálogo SQLite en todas las plataformas.
// It reads existing gateway.sqlite and master.key without migration or reset.
package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	sealer *secretBox
	path   string
}

type AuditEvent struct {
	Time    string `json:"time"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

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

// En Unix, los archivos privados no dan permisos a grupo ni otros. Windows usa
// ACL; la protección del directorio se documenta por separado.
func privateFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0077 == 0)
}
func privateDirectory(info os.FileInfo) bool {
	return info.IsDir() && (runtime.GOOS == "windows" || info.Mode().Perm()&0077 == 0)
}

// Open no crea una clave maestra si encuentra un catálogo cifrado existente.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && !privateFile(info) {
		return nil, errors.New("SQLite debe ser un archivo regular y privado")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // session-level PRAGMAs, predictable transactions
	db.SetMaxIdleConns(1)
	closeFail := func(e error) (*Store, error) { _ = db.Close(); return nil, e }
	// Configura el tiempo de espera de bloqueos antes de consultar esquemas o claves.
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return closeFail(err)
	}
	if err = db.Ping(); err != nil {
		return closeFail(err)
	}
	keyPath := filepath.Join(filepath.Dir(path), "master.key")
	if _, e := os.Stat(keyPath); os.IsNotExist(e) {
		var existing int
		e = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('metadata','gateway_settings','gateway_services')").Scan(&existing)
		if e != nil {
			return closeFail(e)
		}
		if existing > 0 {
			return closeFail(errors.New("master.key no existe para esta base de datos; restaura la copia de seguridad"))
		}
	} else if e != nil {
		return closeFail(e)
	}
	sealer, err := openSecretBox(keyPath)
	if err != nil {
		return closeFail(fmt.Errorf("clave maestra: %w", err))
	}
	s := &Store{db: db, sealer: sealer, path: path}
	if err = os.Chmod(path, 0600); err != nil {
		return closeFail(err)
	}
	var mode string
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return closeFail(err)
	}
	if !strings.EqualFold(mode, "wal") {
		if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			return closeFail(err)
		}
	}

	for _, query := range []string{
		"PRAGMA foreign_keys=ON",
		"CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS audit (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, outcome TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS api_tokens (hash TEXT PRIMARY KEY, label TEXT NOT NULL, role TEXT NOT NULL, scopes TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL)",
	} {
		if _, err = db.Exec(query); err != nil {
			return closeFail(err)
		}
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	e := s.db.Close()
	if e == nil {
		s.db = nil
	}
	return e
}
func (s *Store) PutJSON(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	sealed, err := s.sealer.seal(string(raw))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec("INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, sealed)
	return err
}
func (s *Store) GetJSON(key string, v any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var encoded string
	e := s.db.QueryRow("SELECT value FROM metadata WHERE key=?", key).Scan(&encoded)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	plain, e := s.sealer.open(encoded)
	if e != nil {
		return false, e
	}
	return true, json.Unmarshal([]byte(plain), v)
}
func (s *Store) Audit(actor, action, outcome string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e := s.db.Exec("INSERT INTO audit(happened_at,actor,action,outcome) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), actor, action, outcome)
	return e
}
func (s *Store) ListAudit(limit int) ([]AuditEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT happened_at,actor,action,outcome FROM audit ORDER BY id DESC LIMIT ?", limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var a AuditEvent
		if e = rows.Scan(&a.Time, &a.Actor, &a.Action, &a.Outcome); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) PageAudit(page, size int, sortBy, order string) ([]AuditEvent, int, error) {
	if page < 1 || page > 100000 || size < 1 || size > 50 {
		return nil, 0, errors.New("paginación inválida")
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
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, e := s.db.Query("SELECT happened_at,actor,action,outcome FROM audit ORDER BY "+col+" "+direction+",id DESC LIMIT ? OFFSET ?", size, (page-1)*size)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var a AuditEvent
		if e = rows.Scan(&a.Time, &a.Actor, &a.Action, &a.Outcome); e != nil {
			return nil, 0, e
		}
		out = append(out, a)
	}
	if e = rows.Err(); e != nil {
		return nil, 0, e
	}
	return out, total, nil
}
func (s *Store) InitCatalog() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Un catálogo actualizado no necesita escribir. Leer ACME o estado no debe
	// competir con copias y ajustes ni convertir un bloqueo de lectura en escritura.
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 3 {
		return nil
	}

	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS gateway_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS gateway_services (name TEXT PRIMARY KEY, json TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS gateway_templates (name TEXT PRIMARY KEY, json TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE INDEX IF NOT EXISTS idx_gateway_services_updated ON gateway_services(updated_at)`,
		`PRAGMA user_version=3`,
	} {
		if _, e = tx.Exec(q); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) GetSettings() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT key,value FROM gateway_settings ORDER BY key")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if e = rows.Scan(&k, &v); e != nil {
			return nil, e
		}
		p, ee := s.sealer.open(v)
		if ee != nil {
			return nil, fmt.Errorf("ajuste %s: %w", k, ee)
		}
		out[k] = p
	}
	return out, rows.Err()
}
func (s *Store) ReplaceSettings(settings map[string]string) error {
	// Cifra todos los valores antes de adquirir el bloqueo de escritura.
	encoded := make(map[string]string, len(settings))
	for k, v := range settings {
		if k == "" || len(k) > 128 || len(v) > 16384 {
			return fmt.Errorf("clave o valor de configuración inválido: %q", k)
		}
		cipher, e := s.sealer.seal(v)
		if e != nil {
			return e
		}
		encoded[k] = cipher
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("DELETE FROM gateway_settings"); e != nil {
		return e
	}
	for k, v := range encoded {
		if _, e = tx.Exec("INSERT INTO gateway_settings(key,value) VALUES(?,?)", k, v); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) ListServices() ([]ServiceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT json FROM gateway_services ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ServiceRecord{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		p, er := s.sealer.open(v)
		if er != nil {
			return nil, er
		}
		var r ServiceRecord
		if er = json.Unmarshal([]byte(p), &r); er != nil {
			return nil, er
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) SaveService(item ServiceRecord) error {
	if item.Name == "" || len(item.Name) > 63 || len(item.Backend) > 8192 || len(item.Proxy) > 32768 || len(item.Description) > 1024 {
		return errors.New("tamaño de servicio inválido")
	}
	raw, e := json.Marshal(item)
	if e != nil {
		return e
	}
	sealed, e := s.sealer.seal(string(raw))
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e = s.db.Exec(`INSERT INTO gateway_services(name,json,updated_at) VALUES(?,?,CURRENT_TIMESTAMP) ON CONFLICT(name) DO UPDATE SET json=excluded.json,updated_at=CURRENT_TIMESTAMP`, item.Name, sealed)
	return e
}
func (s *Store) DeleteService(name string) error {
	if name == "" {
		return errors.New("servicio vacío")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e := s.db.Exec("DELETE FROM gateway_services WHERE name=?", name)
	return e
}
func (s *Store) ListTemplates() ([]TemplateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.db.Query("SELECT json FROM gateway_templates ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TemplateRecord{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		p, er := s.sealer.open(v)
		if er != nil {
			return nil, er
		}
		var r TemplateRecord
		if er = json.Unmarshal([]byte(p), &r); er != nil {
			return nil, er
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) SaveTemplate(item TemplateRecord) error {
	if item.Name == "" || len(item.Name) > 63 || len(item.Proxy) > 32768 || len(item.Description) > 1024 || len(item.Category) > 80 || len(item.BackendHint) > 256 || !json.Valid(item.Proxy) {
		return errors.New("plantilla inválida")
	}
	raw, e := json.Marshal(item)
	if e != nil {
		return e
	}
	cipher, e := s.sealer.seal(string(raw))
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e = s.db.Exec(`INSERT INTO gateway_templates(name,json,updated_at) VALUES(?,?,CURRENT_TIMESTAMP) ON CONFLICT(name) DO UPDATE SET json=excluded.json,updated_at=CURRENT_TIMESTAMP`, item.Name, cipher)
	return e
}
func (s *Store) DeleteTemplate(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") {
		return errors.New("plantilla inválida")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, e := s.db.Exec("DELETE FROM gateway_templates WHERE name=?", name)
	return e
}
