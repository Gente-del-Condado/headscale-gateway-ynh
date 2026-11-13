package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/platformfs"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
)

const backupSettingsKey = "backup.schedule.v2"

var backupID = regexp.MustCompile(`^backup-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{8}$`)

// Los componentes son independientes, salvo SQLite, que siempre incluye su clave maestra AES.
type backupComponents struct {
	SQLite   bool `json:"sqlite"`
	ACL      bool `json:"acl"`
	DNS      bool `json:"dns"`
	Identity bool `json:"identity"`
	Full     bool `json:"full"`
}

func (c backupComponents) valid() bool { return c.Full || c.SQLite || c.ACL || c.DNS || c.Identity }

type backupSettings struct {
	IntervalHours int              `json:"interval_hours"`
	Keep          int              `json:"keep"`
	Components    backupComponents `json:"components"`
	LastUTC       string           `json:"last_utc,omitempty"`
}
type backupEntry struct {
	ID         string           `json:"id"`
	CreatedAt  string           `json:"created_at"`
	Components backupComponents `json:"components"`
	ACL        string           `json:"acl"`
	MagicDNS   string           `json:"magic_dns"`
	Bytes      int64            `json:"bytes"`
	SHA256     string           `json:"sha256"`
	Encrypted  bool             `json:"encrypted"`
}
type backupManager struct {
	mu       sync.Mutex
	server   *Server
	settings backupSettings
}

func (s *Server) initBackups() error {
	b := &backupManager{server: s, settings: backupSettings{Keep: 7, Components: backupComponents{SQLite: true}}}
	var stored backupSettings
	if found, e := s.database.GetJSON(backupSettingsKey, &stored); e != nil {
		return e
	} else if found {
		if e = validBackupSettings(stored); e != nil {
			return e
		}
		b.settings = stored
	}
	s.backups = b
	go b.runScheduler()
	return nil
}
func validBackupSettings(p backupSettings) error {
	switch p.IntervalHours {
	case 0, 6, 12, 24, 168:
	default:
		return errors.New("frecuencia no válida")
	}
	if p.Keep < 1 || p.Keep > 30 {
		return errors.New("retención: entre 1 y 30 copias")
	}
	if !p.Components.valid() {
		return errors.New("selecciona al menos un componente")
	}
	return nil
}
func (b *backupManager) runScheduler() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		b.mu.Lock()
		p := b.settings
		if p.IntervalHours > 0 {
			last, _ := time.Parse(time.RFC3339Nano, p.LastUTC)
			if last.IsZero() || time.Since(last) >= time.Duration(p.IntervalHours)*time.Hour {
				if _, e := b.createLocked(context.Background(), p.Components, ""); e != nil {
					diagnostics.Emit("admin", diagnostics.Error, "HGW-BACKUP-ERROR", "programada: %v", e)
				}
			}
		}
		b.mu.Unlock()
	}
}
func secureBackupRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return errors.New("ruta de copias no absoluta")
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(root)
	if e != nil {
		return e
	}
	if !platformfs.PrivateDirectory(info) {
		return errors.New("ruta de copias requiere permisos 0700")
	}
	return nil
}
func writePrivateJSON(path string, v any) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}
func (b *backupManager) root() string { return b.server.database.BackupRoot() }
func (b *backupManager) keyPath() string {
	return filepath.Join(filepath.Dir(b.root()), "backup-recipient.json")
}
func (b *backupManager) createLocked(ctx context.Context, parts backupComponents, password string) (backupEntry, error) {
	var out backupEntry
	if !parts.valid() {
		return out, errors.New("debes seleccionar un componente")
	}
	s := b.server
	if s.database == nil {
		return out, errors.New("SQLite no disponible")
	}
	root := b.root()
	if e := secureBackupRoot(root); e != nil {
		return out, e
	}
	k, e := securebackup.ReadKey(b.keyPath())
	if e != nil {
		return out, errors.New("primero configura la contraseña de cifrado de copias")
	}
	if password != "" {
		if e = securebackup.CheckPassword(b.keyPath(), password); e != nil {
			return out, e
		}
	}
	stage, e := os.MkdirTemp(root, ".stage-")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(stage)
	dataDir := filepath.Dir(root)
	if parts.Full || parts.Identity {
		if ctl, ok := s.controller.(interface{ BackupDataDir() string }); ok {
			configured := ctl.BackupDataDir()
			if configured != "" && filepath.Clean(configured) != filepath.Clean(dataDir) {
				return out, errors.New("para respaldar/restaurar data_dir e identidad, este modo requiere que SQLite y data_dir estén en la misma carpeta")
			}
		}
	}
	files := map[string]string{}
	if parts.Full {
		files, e = securebackup.ScanDirectory(dataDir, nil)
		if e != nil {
			return out, e
		}
	}
	if parts.Identity && !parts.Full {
		files, e = securebackup.ScanDirectory(dataDir, func(name string) bool {
			return name == "identity" || strings.HasPrefix(name, "identity/") || strings.HasPrefix(name, "tailscale-state-") || strings.HasPrefix(name, "tailscale-")
		})
		if e != nil {
			return out, e
		}
		if len(files) == 0 {
			return out, errors.New("no hay estado de identidad tsnet que respaldar")
		}
	}
	if parts.SQLite || parts.Full {
		sqlDir := filepath.Join(stage, "database")
		if e = s.database.BackupTo(sqlDir); e != nil {
			return out, e
		}
		files["data/gateway.sqlite"] = filepath.Join(sqlDir, "gateway.sqlite")
		files["data/master.key"] = filepath.Join(sqlDir, "master.key")
		delete(files, "data/gateway.sqlite-wal")
		delete(files, "data/gateway.sqlite-shm")
	}
	aclStatus, dnsStatus := "not_requested", "not_requested"
	if parts.ACL || parts.DNS || parts.Full {
		ctl, ok := s.controller.(HeadscalePolicyDNSController)
		if !ok {
			return out, errors.New("controlador Headscale no disponible")
		}
		checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if parts.ACL || parts.Full {
			policy, err := ctl.HeadscaleGetPolicy(checkCtx)
			if err != nil {
				return out, fmt.Errorf("ACL: %w", err)
			}
			p := filepath.Join(stage, "headscale-acl.json")
			if e = writePrivateJSON(p, policy); e != nil {
				return out, e
			}
			files["remote/headscale-acl.json"] = p
			aclStatus = "included"
			if policy.Missing {
				aclStatus = "missing"
			}
		}
		if parts.DNS || parts.Full {
			records, err := ctl.HeadscaleServiceDNS(checkCtx)
			if err != nil {
				return out, fmt.Errorf("DNS: %w", err)
			}
			p := filepath.Join(stage, "magicdns-export.json")
			if e = writePrivateJSON(p, records); e != nil {
				return out, e
			}
			files["remote/magicdns-export.json"] = p
			dnsStatus = "generated_export"
		}
	}
	if len(files) == 0 {
		return out, errors.New("no hay archivos para guardar")
	}
	var suffix [4]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		return out, e
	}
	id := "backup-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix[:])
	dest := filepath.Join(root, id+".hgw")
	manifest, e := securebackup.Pack(ctx, dest, k, map[bool]string{true: "all", false: "selected"}[parts.Full], files)
	if e != nil {
		return out, e
	}
	f, e := os.Open(dest)
	if e != nil {
		return out, e
	}
	sum := sha256.New()
	_, e = io.Copy(sum, f)
	f.Close()
	if e != nil {
		return out, e
	}
	st, e := os.Stat(dest)
	if e != nil {
		return out, e
	}
	out = backupEntry{ID: id, CreatedAt: manifest.Created, Components: parts, ACL: aclStatus, MagicDNS: dnsStatus, Bytes: st.Size(), SHA256: hex.EncodeToString(sum.Sum(nil)), Encrypted: true}
	if e = writePrivateJSON(filepath.Join(root, id+".json"), out); e != nil {
		_ = os.Remove(dest)
		return backupEntry{}, e
	}
	b.settings.LastUTC = manifest.Created
	if e = s.database.PutJSON(backupSettingsKey, b.settings); e != nil {
		return out, e
	}
	if e = b.pruneLocked(); e != nil {
		diagnostics.Emit("admin", diagnostics.Warn, "HGW-BACKUP-RETENTION", "retención: %v", e)
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-BACKUP-OK", "copia cifrada=%s bytes=%d sha256=%s", id, out.Bytes, out.SHA256)
	return out, nil
}
func (b *backupManager) entry(id string) (backupEntry, error) {
	var item backupEntry
	if !backupID.MatchString(id) {
		return item, errors.New("identificador inválido")
	}
	raw, e := os.ReadFile(filepath.Join(b.root(), id+".json"))
	if e != nil {
		return item, e
	}
	if e = json.Unmarshal(raw, &item); e != nil {
		return item, e
	}
	if item.ID != id || !item.Encrypted || len(item.SHA256) != 64 {
		return item, errors.New("metadatos inválidos")
	}
	info, e := os.Lstat(filepath.Join(b.root(), id+".hgw"))
	if e != nil {
		return item, e
	}
	if !platformfs.PrivateFile(info) {
		return item, errors.New("copia insegura")
	}
	return item, nil
}
func (b *backupManager) listLocked() ([]backupEntry, error) {
	if e := secureBackupRoot(b.root()); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(b.root())
	if e != nil {
		return nil, e
	}
	out := []backupEntry{}
	for _, i := range entries {
		if !strings.HasSuffix(i.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(i.Name(), ".json")
		if !backupID.MatchString(id) {
			continue
		}
		if item, e := b.entry(id); e == nil {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
func (b *backupManager) pruneLocked() error {
	entries, e := b.listLocked()
	if e != nil {
		return e
	}
	for _, item := range entries[min(len(entries), b.settings.Keep):] {
		if e = os.Remove(filepath.Join(b.root(), item.ID+".hgw")); e != nil {
			return e
		}
		if e = os.Remove(filepath.Join(b.root(), item.ID+".json")); e != nil {
			return e
		}
	}
	return nil
}
func setBackupTransferDeadlines(w http.ResponseWriter, upload bool) error {
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(2 * time.Hour)
	if upload {
		if err := controller.SetReadDeadline(deadline); err != nil {
			return err
		}
	}
	return controller.SetWriteDeadline(deadline)
}

func (b *backupManager) archive(w http.ResponseWriter, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, e := b.entry(id)
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.hgw"`, item.ID))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	f, e := os.Open(filepath.Join(b.root(), id+".hgw"))
	if e != nil {
		return e
	}
	defer f.Close()
	http.ServeContent(w, &http.Request{Method: http.MethodGet, URL: &url.URL{}}, item.ID+".hgw", time.Now(), f)
	return nil
}
func (s *Server) registerBackupRoutes(mux routeRegistrar) {
	onlyAdmin := func(fn http.HandlerFunc) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || s.auth == nil || !s.auth.sessionAllowed(r) {
				problem(w, 403, "las copias requieren sesión interactiva de administrador")
				return
			}
			if s.backups == nil {
				problem(w, 503, "copias no disponibles")
				return
			}
			fn(w, r)
		})
	}
	mux.HandleFunc("GET /api/v1/backups", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		entries, e := b.listLocked()
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		_, keyErr := securebackup.ReadKey(b.keyPath())
		reply(w, 200, map[string]any{"items": entries, "settings": b.settings, "encryption_ready": keyErr == nil}, nil)
	}))
	mux.HandleFunc("GET /api/v1/backups/restore/status", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		// Guarda el estado en el padre de data_dir: la restauración sustituye data_dir,
		// pero conserva el último resultado para que pueda consultarlo el navegador.
		path := filepath.Join(filepath.Dir(filepath.Dir(s.backups.root())), ".hgw-restore-status.json")
		raw, e := os.ReadFile(path)
		if os.IsNotExist(e) {
			reply(w, 200, map[string]any{"phase": "idle"}, nil)
			return
		}
		if e != nil || len(raw) > 8192 {
			problem(w, 500, "estado de restauración no disponible")
			return
		}
		var result struct {
			Job     string `json:"job"`
			Phase   string `json:"phase"`
			Detail  string `json:"detail"`
			Updated string `json:"updated_at"`
		}
		if json.Unmarshal(raw, &result) != nil {
			problem(w, 500, "estado inválido")
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("POST /api/v1/backups/encryption", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, e := os.Stat(b.keyPath()); e == nil {
			problem(w, 409, "la contraseña ya está configurada; no se puede sobrescribir sin migrar las copias")
			return
		} else if !os.IsNotExist(e) {
			problem(w, 500, "no se pudo comprobar clave")
			return
		}
		if e := securebackup.NewKey(b.keyPath(), body.Password); e != nil {
			problem(w, 400, e.Error())
			return
		}
		reply(w, 200, map[string]any{"configured": true}, nil)
	}))
	mux.HandleFunc("PUT /api/v1/backups/schedule", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var p backupSettings
		if !decode(w, r, &p) {
			return
		}
		if e := validBackupSettings(p); e != nil {
			problem(w, 400, e.Error())
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		if p.IntervalHours > 0 {
			if _, e := securebackup.ReadKey(b.keyPath()); e != nil {
				problem(w, 409, "configura primero la contraseña de copias")
				return
			}
		}
		p.LastUTC = b.settings.LastUTC
		if e := s.database.PutJSON(backupSettingsKey, p); e != nil {
			problem(w, 500, "no se guardó programación")
			return
		}
		b.settings = p
		reply(w, 200, p, nil)
	}))
	mux.HandleFunc("POST /api/v1/backups", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password   string           `json:"password"`
			Components backupComponents `json:"components"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Password == "" {
			problem(w, 400, "contraseña requerida para crear copia manual")
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		item, e := b.createLocked(r.Context(), body.Components, body.Password)
		if e != nil {
			problem(w, 400, e.Error())
			return
		}
		reply(w, 201, item, nil)
	}))
	mux.HandleFunc("GET /api/v1/backups/{id}/download", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		if err := setBackupTransferDeadlines(w, false); err != nil {
			problem(w, http.StatusInternalServerError, "No se pudo ampliar el plazo de transferencia de la copia")
			return
		}
		if e := s.backups.archive(w, r.PathValue("id")); e != nil {
			problem(w, 404, "copia no encontrada")
		}
	}))
	mux.HandleFunc("DELETE /api/v1/backups/{id}", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		id := r.PathValue("id")
		if _, e := b.entry(id); e != nil {
			problem(w, 404, "copia inexistente")
			return
		}
		if e := os.Remove(filepath.Join(b.root(), id+".hgw")); e != nil {
			problem(w, 500, e.Error())
			return
		}
		_ = os.Remove(filepath.Join(b.root(), id+".json"))
		reply(w, 200, map[string]any{"deleted": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/backups/import", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		// Recibe en flujo una copia .hgw cifrada desde el equipo del administrador.
		// Nunca acepta ni guarda contenido sin cifrar como copia de seguridad.
		if err := setBackupTransferDeadlines(w, true); err != nil {
			problem(w, http.StatusInternalServerError, "No se pudo ampliar el plazo de transferencia de la copia")
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		if err := secureBackupRoot(b.root()); err != nil {
			problem(w, 500, err.Error())
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 50<<30)
		mr, err := r.MultipartReader()
		if err != nil {
			problem(w, 400, "subida multipart inválida")
			return
		}
		var password string
		var saved string
		defer func() {
			if saved != "" {
				_ = os.Remove(saved)
			}
		}()
		for {
			part, e := mr.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				problem(w, 400, "subida incompleta")
				return
			}
			switch part.FormName() {
			case "password":
				raw, e := io.ReadAll(io.LimitReader(part, 4096))
				if e != nil {
					problem(w, 400, "contraseña ilegible")
					return
				}
				password = string(raw)
			case "archive":
				if saved != "" {
					problem(w, 400, "solo un archivo permitido")
					return
				}
				f, e := os.CreateTemp(b.root(), ".uploaded-*.hgw")
				if e != nil {
					problem(w, 500, "sin espacio para la subida")
					return
				}
				saved = f.Name()
				_ = f.Chmod(0600)
				_, e = io.Copy(f, part)
				if e == nil {
					e = f.Sync()
				}
				if ce := f.Close(); e == nil {
					e = ce
				}
				if e != nil {
					problem(w, 400, "archivo incompleto")
					return
				}
			default:
				problem(w, 400, "campo no permitido")
				return
			}
			_ = part.Close()
		}
		b.importArchiveLocked(w, r, saved, password)
	}))
	mux.HandleFunc("POST /api/v1/backups/{id}/restore-acl", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
			Confirm  string `json:"confirm"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Password == "" || body.Confirm != "RESTAURAR ACL" {
			problem(w, 400, "contraseña y confirmación explícita necesarias")
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		item, e := b.entry(r.PathValue("id"))
		if e != nil || !item.Components.ACL && !item.Components.Full {
			problem(w, 404, "copia sin ACL disponible")
			return
		}
		ctl, ok := s.controller.(HeadscalePolicyDNSController)
		if !ok {
			problem(w, 501, "controlador ACL no disponible")
			return
		}
		stage, e := os.MkdirTemp(b.root(), ".acl-verify-")
		if e != nil {
			problem(w, 500, "no se pudo preparar verificación")
			return
		}
		defer os.RemoveAll(stage)
		_, e = securebackup.Extract(r.Context(), filepath.Join(b.root(), item.ID+".hgw"), body.Password, stage)
		if e != nil {
			problem(w, 400, "copia o contraseña inválida")
			return
		}
		var snapshot struct {
			Policy  string `json:"policy"`
			Missing bool   `json:"missing"`
		}
		raw, e := os.ReadFile(filepath.Join(stage, "remote", "headscale-acl.json"))
		if e != nil || json.Unmarshal(raw, &snapshot) != nil || snapshot.Missing || snapshot.Policy == "" {
			problem(w, 409, "la instantánea no contiene una política ACL restaurable")
			return
		}
		current, e := ctl.HeadscaleGetPolicy(r.Context())
		if e != nil {
			problem(w, 502, "no se pudo leer la ACL actual: "+e.Error())
			return
		}
		_, e = ctl.HeadscaleSetPolicy(r.Context(), snapshot.Policy, current.Revision)
		if e != nil {
			problem(w, 409, "Headscale rechazó la restauración ACL: "+e.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ACL-RESTORE", "política restaurada explícitamente desde %s", item.ID)
		reply(w, 200, map[string]any{"restored": true, "component": "acl"}, nil)
	}))
	mux.HandleFunc("POST /api/v1/backups/{id}/dns-export", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Password == "" {
			problem(w, 400, "contraseña necesaria")
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		item, e := b.entry(r.PathValue("id"))
		if e != nil || !item.Components.DNS && !item.Components.Full {
			problem(w, 404, "copia sin registros DNS")
			return
		}
		stage, e := os.MkdirTemp(b.root(), ".dns-verify-")
		if e != nil {
			problem(w, 500, "no se pudo preparar verificación")
			return
		}
		defer os.RemoveAll(stage)
		_, e = securebackup.Extract(r.Context(), filepath.Join(b.root(), item.ID+".hgw"), body.Password, stage)
		if e != nil {
			problem(w, 400, "copia o contraseña inválida")
			return
		}
		raw, e := os.ReadFile(filepath.Join(stage, "remote", "magicdns-export.json"))
		if e != nil {
			problem(w, 409, "no hay instantánea DNS")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="magicdns-extra-records.json"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(raw)
	}))
	// Prepara y valida desde el panel; sustituye los datos al reejecutar el proceso,
	// cuando ni tsnet ni SQLite están en marcha.
	mux.HandleFunc("POST /api/v1/backups/{id}/restore", onlyAdmin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
			Confirm  string `json:"confirm"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Confirm != "RESTAURAR" || body.Password == "" {
			problem(w, 400, "confirmación y contraseña obligatorias")
			return
		}
		b := s.backups
		b.mu.Lock()
		defer b.mu.Unlock()
		id := r.PathValue("id")
		item, e := b.entry(id)
		if e != nil {
			problem(w, 404, "copia inexistente")
			return
		}
		if !item.Components.SQLite && !item.Components.Identity && !item.Components.Full {
			problem(w, 409, "esta copia solo contiene datos remotos; no se restaura automáticamente en Headscale")
			return
		}
		ctl, ok := s.controller.(interface {
			ScheduleBackupRestore(context.Context, string, string) (string, error)
		})
		if !ok {
			problem(w, 501, "restauración integrada no disponible en este sistema")
			return
		}
		job, e := ctl.ScheduleBackupRestore(r.Context(), filepath.Join(b.root(), id+".hgw"), body.Password)
		if e != nil {
			problem(w, 409, e.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-RESTORE-STAGED", "copia=%s job=%s", id, job)
		reply(w, 202, map[string]any{"restarting": true, "job": job}, nil)
	}))
}

// importArchiveLocked valida y registra una copia recibida en un archivo temporal privado.
// El llamante mantiene el bloqueo de copias y elimina el temporal al terminar.
func (b *backupManager) importArchiveLocked(w http.ResponseWriter, r *http.Request, saved, password string) {
	if saved == "" || password == "" {
		problem(w, 400, "archivo y contraseña requeridos")
		return
	}
	stage, e := os.MkdirTemp(b.root(), ".verify-")
	if e != nil {
		problem(w, 500, "no se pudo crear verificación")
		return
	}
	defer os.RemoveAll(stage)
	manifest, e := securebackup.Extract(r.Context(), saved, password, stage)
	if e != nil {
		problem(w, 400, "no se pudo verificar integridad/contraseña: "+e.Error())
		return
	}
	components := backupComponents{}
	for name := range manifest.Files {
		switch {
		case name == "data/gateway.sqlite":
			components.SQLite = true
		case name == "remote/headscale-acl.json":
			components.ACL = true
		case name == "remote/magicdns-export.json":
			components.DNS = true
		case strings.HasPrefix(name, "data/tailscale-state-") || strings.HasPrefix(name, "data/identity/"):
			components.Identity = true
		}
	}
	components.Full = manifest.Mode == "all"
	if !components.valid() {
		problem(w, 400, "copia no contiene componentes reconocidos")
		return
	}
	var suffix [4]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		problem(w, 500, "error aleatorio")
		return
	}
	id := "backup-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix[:])
	file, e := os.Open(saved)
	if e != nil {
		problem(w, 500, "error de archivo")
		return
	}
	hash := sha256.New()
	_, e = io.Copy(hash, file)
	file.Close()
	if e != nil {
		problem(w, 500, "error SHA-256")
		return
	}
	stat, e := os.Stat(saved)
	if e != nil {
		problem(w, 500, "error de archivo")
		return
	}
	item := backupEntry{ID: id, CreatedAt: manifest.Created, Components: components, ACL: map[bool]string{true: "included", false: "not_requested"}[components.ACL], MagicDNS: map[bool]string{true: "generated_export", false: "not_requested"}[components.DNS], Bytes: stat.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)), Encrypted: true}
	dest := filepath.Join(b.root(), id+".hgw")
	if e = os.Rename(saved, dest); e != nil {
		problem(w, 500, "no se guardó archivo")
		return
	}
	if e = writePrivateJSON(filepath.Join(b.root(), id+".json"), item); e != nil {
		_ = os.Remove(dest)
		problem(w, 500, "no se guardaron metadatos")
		return
	}
	if e = b.pruneLocked(); e != nil {
		diagnostics.Emit("admin", diagnostics.Warn, "HGW-BACKUP-RETENTION", "retención: %v", e)
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-BACKUP-IMPORT", "archivo cifrado=%s", id)
	reply(w, 201, item, nil)
}
