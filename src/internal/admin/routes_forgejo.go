package admin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gentedelcondado/headscale-gateway/internal/forgejostore"
	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
)

type forgejoCredentials struct {
	Repository string `json:"repository"`
	Token      string `json:"token"`
}

func (c forgejoCredentials) store(w http.ResponseWriter) *forgejostore.Store {
	store, err := forgejostore.New(c.Repository, c.Token)
	if err != nil {
		problem(w, 400, err.Error())
	}
	return store
}

func (s *Server) registerForgejoRoutes(mux routeRegistrar) {
	gate := func(fn http.HandlerFunc) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if s.requirePKISession(w, r) {
				w.Header().Set("Cache-Control", "no-store")
				fn(w, r)
			}
		})
	}

	mux.HandleFunc("POST /api/v1/forgejo/list", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			forgejoCredentials
			Page int `json:"page"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		store := body.store(w)
		if store == nil {
			return
		}
		items, more, err := store.List(r.Context(), body.Page)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"items": items, "more": more}, nil)
	}))

	mux.HandleFunc("POST /api/v1/forgejo/profile/export", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			forgejoCredentials
			Password string `json:"password"`
			Secrets  bool   `json:"secrets"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		store := body.store(w)
		if store == nil {
			return
		}
		controller, ok := s.controller.(configurationTransfer)
		if !ok {
			problem(w, 501, "exportación no disponible")
			return
		}
		data, err := controller.ExportConfiguration(r.Context(), body.Secrets, body.Password)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		digest := sha256.Sum256(data)
		item, err := store.Upload(r.Context(), forgejostore.Profile, bytes.NewReader(data), int64(len(data)), hex.EncodeToString(digest[:]))
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 201, item, nil)
	}))

	mux.HandleFunc("POST /api/v1/forgejo/profile/import", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			forgejoCredentials
			Tag      string `json:"tag"`
			Password string `json:"password"`
			Apply    bool   `json:"apply"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		store := body.store(w)
		if store == nil {
			return
		}
		controller, ok := s.controller.(interface {
			configurationTransfer
			PreviewConfiguration([]byte, string) ([]string, error)
		})
		if !ok {
			problem(w, 501, "importación no disponible")
			return
		}
		if body.Password == "" {
			problem(w, 400, "archivo y contraseña requeridos")
			return
		}
		var data bytes.Buffer
		if _, err := store.Download(r.Context(), body.Tag, forgejostore.Profile, &data); err != nil {
			problem(w, 502, err.Error())
			return
		}
		if !body.Apply {
			keys, err := controller.PreviewConfiguration(data.Bytes(), body.Password)
			if err != nil {
				problem(w, 400, err.Error())
				return
			}
			reply(w, 200, map[string]any{"keys": keys}, nil)
			return
		}
		count, err := controller.ImportConfiguration(r.Context(), data.Bytes(), body.Password)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"imported": count, "restart_required": true}, nil)
	}))

	mux.HandleFunc("POST /api/v1/forgejo/backup/export", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			forgejoCredentials
			ID string `json:"id"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		store := body.store(w)
		if store == nil || !s.prepareForgejoBackup(w) {
			return
		}
		b := s.backups
		b.mu.Lock()
		entry, err := b.entry(body.ID)
		if err != nil {
			b.mu.Unlock()
			problem(w, 404, "copia inexistente")
			return
		}
		file, err := os.Open(filepath.Join(b.root(), entry.ID+".hgw"))
		b.mu.Unlock()
		if err != nil {
			problem(w, 500, "error de archivo")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || entry.Bytes <= 0 || entry.Bytes > forgejostore.MaxBackup || info.Size() != entry.Bytes || !securebackup.HasEncryptedHeader(file) {
			problem(w, 400, "archivo cifrado inválido")
			return
		}
		if _, err := file.Seek(0, 0); err != nil {
			problem(w, 500, "error de archivo")
			return
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, io.LimitReader(file, entry.Bytes+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			problem(w, 400, "la copia local no coincide con su SHA-256")
			return
		}
		if _, err := file.Seek(0, 0); err != nil {
			problem(w, 500, "error de archivo")
			return
		}
		item, err := store.Upload(r.Context(), forgejostore.Backup, file, entry.Bytes, entry.SHA256)
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 201, item, nil)
	}))

	mux.HandleFunc("POST /api/v1/forgejo/backup/import", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			forgejoCredentials
			Tag      string `json:"tag"`
			Password string `json:"password"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		store := body.store(w)
		if store == nil || !s.prepareForgejoBackup(w) {
			return
		}
		if body.Password == "" {
			problem(w, 400, "archivo y contraseña requeridos")
			return
		}
		b := s.backups
		if err := secureBackupRoot(b.root()); err != nil {
			problem(w, 500, "no se pudo preparar el directorio de copias")
			return
		}
		file, err := os.CreateTemp(b.root(), ".forgejo-*.hgw")
		if err != nil {
			problem(w, 500, "sin espacio para la subida")
			return
		}
		defer os.Remove(file.Name())
		_, err = store.Download(r.Context(), body.Tag, forgejostore.Backup, file)
		closeErr := file.Close()
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		if closeErr != nil {
			problem(w, 500, "archivo incompleto")
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.importArchiveLocked(w, r, file.Name(), body.Password)
	}))
}

func (s *Server) prepareForgejoBackup(w http.ResponseWriter) bool {
	if s.backups == nil {
		problem(w, 503, "copias no disponibles")
		return false
	}
	if err := setBackupTransferDeadlines(w, false); err != nil {
		problem(w, 500, "No se pudo ampliar el plazo de transferencia de la copia")
		return false
	}
	return true
}
