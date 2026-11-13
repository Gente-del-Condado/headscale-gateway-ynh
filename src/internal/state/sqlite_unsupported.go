//go:build legacy_sqlite && (!linux || !cgo)

package state

import (
	"errors"
	"time"
)

var ErrUnavailable = errors.New("SQLite requiere Linux y CGO_ENABLED=1 (libsqlite3-dev/sqlite-devel)")

type Store struct{}
type AuditEvent struct{ Time, Actor, Action, Outcome string }

func Open(string) (*Store, error)                  { return nil, ErrUnavailable }
func (*Store) Close() error                        { return nil }
func (*Store) PutJSON(string, any) error           { return ErrUnavailable }
func (*Store) GetJSON(string, any) (bool, error)   { return false, ErrUnavailable }
func (*Store) Audit(string, string, string) error  { return ErrUnavailable }
func (*Store) ListAudit(int) ([]AuditEvent, error) { return nil, ErrUnavailable }
func (*Store) PageAudit(int, int, string, string) ([]AuditEvent, int, error) {
	return nil, 0, ErrUnavailable
}

type TokenInfo struct {
	ID, Label, Role, Created, Expires string
	Scopes                            []string
}

func (*Store) NewToken(string, string, []string, time.Duration) (string, TokenInfo, error) {
	return "", TokenInfo{}, ErrUnavailable
}
func (*Store) ResolveToken(string) (TokenInfo, bool, error) {
	return TokenInfo{}, false, ErrUnavailable
}
func (*Store) ListTokens() ([]TokenInfo, error) { return nil, ErrUnavailable }
func (*Store) RevokeToken(string) error         { return ErrUnavailable }

type ServiceRecord struct {
	Description       string
	Name              string
	Enabled           bool
	Backend, Template string
	Proxy             []byte
}
type TemplateRecord struct {
	Name, Category, Description, BackendHint string
	Proxy                                    []byte
}

func (*Store) InitCatalog() error                       { return ErrUnavailable }
func (*Store) GetSettings() (map[string]string, error)  { return nil, ErrUnavailable }
func (*Store) ReplaceSettings(map[string]string) error  { return ErrUnavailable }
func (*Store) ListServices() ([]ServiceRecord, error)   { return nil, ErrUnavailable }
func (*Store) SaveService(ServiceRecord) error          { return ErrUnavailable }
func (*Store) DeleteService(string) error               { return ErrUnavailable }
func (*Store) ListTemplates() ([]TemplateRecord, error) { return nil, ErrUnavailable }
func (*Store) SaveTemplate(TemplateRecord) error        { return ErrUnavailable }
func (*Store) DeleteTemplate(string) error              { return ErrUnavailable }

func (*Store) BackupTo(string) error                { return ErrUnavailable }
func (*Store) BackupRoot() string                   { return "" }
func RestoreOffline(string, string) (string, error) { return "", ErrUnavailable }
