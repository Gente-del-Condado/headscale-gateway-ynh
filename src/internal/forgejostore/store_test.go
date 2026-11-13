package forgejostore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStoreRejectsPublicRepository(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forgejo/api/v1/repos/group/repo" || r.Method != http.MethodGet {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"private":false}`)
	}))
	defer server.Close()
	store, err := New(server.URL+"/forgejo/group/repo.git", "token-ficticio")
	if err != nil {
		t.Fatal(err)
	}
	store.client.Transport = server.Client().Transport
	if _, _, err := store.List(t.Context(), 1); err == nil || !strings.Contains(err.Error(), "privado") {
		t.Fatalf("repositorio público: %v", err)
	}
}

func TestDownloadPrivateAttachment(t *testing.T) {
	const contents = "archivo cifrado ficticio"
	digest := sha256.Sum256([]byte(contents))
	checksum := hex.EncodeToString(digest[:])
	tag := tagFor(Profile, checksum)
	for _, test := range []struct {
		name, payload, redirect string
		wantError               bool
	}{
		{"correcto", contents, "/forgejo/group/repo/releases/attachments/abc-123", false},
		{"contenido alterado", strings.Repeat("x", len(contents)), "", true},
		{"destino ajeno", contents, "https://otro.example.invalid/attachments/abc-123", true},
		{"inicio de sesión", contents, "/yunohost/sso", true},
		{"fuera del prefijo", contents, "/attachments/abc-123", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "token token-lectura" {
					t.Error("la recuperación requiere solo solicitudes autenticadas de lectura")
				}
				switch r.URL.Path {
				case "/forgejo/api/v1/repos/group/repo":
					fmt.Fprint(w, `{"private":true}`)
				case "/forgejo/api/v1/repos/group/repo/releases/tags/" + tag:
					_ = json.NewEncoder(w).Encode(map[string]any{
						"tag_name": tag, "body": marker + checksum,
						"assets": []map[string]any{{"name": Profile, "size": len(contents)}},
					})
				case "/forgejo/group/repo/releases/download/" + tag + "/" + Profile:
					if test.redirect != "" {
						http.Redirect(w, r, test.redirect, http.StatusFound)
						return
					}
					fmt.Fprint(w, test.payload)
				case "/forgejo/group/repo/releases/attachments/abc-123":
					fmt.Fprint(w, test.payload)
				default:
					t.Errorf("ruta inesperada: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			store, err := New(server.URL+"/forgejo/group/repo", "token-lectura")
			if err != nil {
				t.Fatal(err)
			}
			store.client.Transport = server.Client().Transport
			var received bytes.Buffer
			_, err = store.Download(t.Context(), tag, Profile, &received)
			if (err != nil) != test.wantError || (!test.wantError && received.String() != contents) {
				t.Fatalf("descarga inesperada: %v", err)
			}
		})
	}
}
