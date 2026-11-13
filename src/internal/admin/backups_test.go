package admin

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func TestEncryptedBackupCreationRetentionAndRestore(t *testing.T) {
	dir := t.TempDir()
	db, e := state.Open(filepath.Join(dir, "gateway.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.PutJSON("test", map[string]string{"value": "secret"}); e != nil {
		t.Fatal(e)
	}
	s := &Server{database: db}
	b := &backupManager{server: s, settings: backupSettings{Keep: 2, Components: backupComponents{SQLite: true}}}
	if e = securebackup.NewKey(b.keyPath(), "contraseña segura de prueba"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		item, e := b.createLocked(context.Background(), b.settings.Components, "contraseña segura de prueba")
		if e != nil {
			t.Fatal(e)
		}
		if !backupID.MatchString(item.ID) || !item.Encrypted {
			t.Fatalf("invalid backup %+v", item)
		}
	}
	items, e := b.listLocked()
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 2 {
		t.Fatalf("retención esperada 2: %d", len(items))
	}
	for _, item := range items {
		f, e := os.Open(filepath.Join(b.root(), item.ID+".hgw"))
		if e != nil {
			t.Fatal(e)
		}
		header := make([]byte, 8)
		_, e = io.ReadFull(f, header)
		f.Close()
		if e != nil || string(header) != "HGWBKP02" {
			t.Fatal("archive not encrypted")
		}
		stage := filepath.Join(t.TempDir(), "stage")
		os.Mkdir(stage, 0700)
		if _, e = securebackup.Extract(context.Background(), filepath.Join(b.root(), item.ID+".hgw"), "contraseña segura de prueba", stage); e != nil {
			t.Fatal(e)
		}
		if _, e = os.Stat(filepath.Join(stage, "data", "master.key")); e != nil {
			t.Fatal(e)
		}
		restored, err := state.Open(filepath.Join(stage, "data", "gateway.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]string
		found, err := restored.GetJSON("test", &value)
		restored.Close()
		if err != nil || !found || value["value"] != "secret" {
			t.Fatalf("restored encrypted settings unavailable: found=%v error=%v", found, err)
		}
	}
}
func TestBackupIDPathTraversalRejected(t *testing.T) {
	for _, id := range []string{"../outside", "backup-20261023T130000Z-12345678", "backup-20261023T130000.000000000Z-abcdef01/../../etc"} {
		if backupID.MatchString(id) {
			t.Fatalf("path traversal accepted: %s", id)
		}
	}
	if !strings.HasPrefix(backupSettingsKey, "backup.") {
		t.Fatal("wrong config key")
	}
}

type backupDeadlineRecorder struct {
	*httptest.ResponseRecorder
	read, write time.Time
}

func (w *backupDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	w.read = deadline
	return nil
}

func (w *backupDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.write = deadline
	return nil
}

func TestBackupTransferSessionAndDeadlines(t *testing.T) {
	dir := t.TempDir()
	db, e := state.Open(filepath.Join(dir, "gateway.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	// Un token Bearer válido no permite descargar copias: se exige una
	// sesión interactiva de administrador. New inicializa el hash del token.
	token := strings.Repeat("b", 64)
	s, e := New(token, &fakeController{})
	if e != nil {
		t.Fatal(e)
	}
	s.database = db
	s.auth = emptyAuthManager("")
	b := &backupManager{server: s, settings: backupSettings{Keep: 2, Components: backupComponents{SQLite: true}}}
	s.backups = b
	if e = securebackup.NewKey(b.keyPath(), "contraseña segura de prueba"); e != nil {
		t.Fatal(e)
	}
	item, e := b.createLocked(context.Background(), b.settings.Components, "")
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	s.registerBackupRoutes(mux)
	blocked := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/"+item.ID+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mux.ServeHTTP(blocked, req)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("Bearer válido no debe descargar copias: HTTP %d", blocked.Code)
	}
	// Una credencial inexistente o errónea debe recibir 401, no 403.
	invalid := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/backups/"+item.ID+"/download", nil)
	invalidReq.Header.Set("Authorization", "Bearer fake")
	mux.ServeHTTP(invalid, invalidReq)
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("Bearer inválido: HTTP %d; esperado 401", invalid.Code)
	}

	handler := s.Handler()
	login := httptest.NewRecorder()
	csrf := s.auth.CreateSession(login, req, "admin")
	cookie := login.Result().Cookies()[0]
	get := httptest.NewRequest(http.MethodGet, "/api/v1/backups/"+item.ID+"/download", nil)
	get.AddCookie(cookie)
	download := &backupDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	earliest := time.Now().Add(2 * time.Hour)
	handler.ServeHTTP(download, get)
	if download.Code != http.StatusOK || download.write.Before(earliest) || !download.read.IsZero() {
		t.Fatalf("descarga HTTP %d, plazos de lectura %v y escritura %v", download.Code, download.read, download.write)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("password", "contraseña segura de prueba"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("archive", "copia.hgw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(part, download.Body); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/backups/import", &body)
	post.AddCookie(cookie)
	post.Header.Set("Content-Type", form.FormDataContentType())
	post.Header.Set("Origin", "http://"+post.Host)
	post.Header.Set("X-CSRF-Token", csrf)
	upload := &backupDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	earliest = time.Now().Add(2 * time.Hour)
	handler.ServeHTTP(upload, post)
	if upload.Code != http.StatusCreated || upload.read.Before(earliest) || upload.write.Before(earliest) {
		t.Fatalf("importación HTTP %d (%s), plazos de lectura %v y escritura %v", upload.Code, upload.Body, upload.read, upload.write)
	}
}

func TestSecureSinTokenInicializadoNoProvocaPanic(t *testing.T) {
	// Las pruebas auxiliares pueden construir el servidor sin New. En tal
	// caso, cualquier credencial debe ser rechazada, nunca desreferenciada.
	s := &Server{}
	called := false
	handler := s.secure(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	for _, header := range []string{"", "Bearer fake", "Bearer " + strings.Repeat("a", 64)} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("cabecera %q: código %d, esperado 401", header, w.Code)
		}
	}
	if called {
		t.Fatal("el manejador protegido se ejecutó sin autenticación")
	}
}
