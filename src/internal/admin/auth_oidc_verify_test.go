package admin

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Comprueba el recorrido real discovery → JWKS → verificación después de
// actualizar go-jose, sin depender de un proveedor ni credenciales reales.
func TestOIDCVerificationWithRSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	provider, _, err := oidcClient(t.Context(), OIDCSettings{Issuer: issuer, ClientID: "test-client"})
	if err != nil {
		t.Fatal(err)
	}
	encode := func(value any) string {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	header := encode(map[string]string{"alg": "RS256", "kid": "test"})
	claims := encode(map[string]any{
		"iss": issuer, "sub": "synthetic-user", "aud": "test-client",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	unsigned := header + "." + claims
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: "test-client"})
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	verified, err := verifier.Verify(t.Context(), token)
	if err != nil || verified.Subject != "synthetic-user" {
		t.Fatalf("verificación RSA incompatible: %v", err)
	}
	signature[0] ^= 1
	if _, err := verifier.Verify(t.Context(), unsigned+"."+base64.RawURLEncoding.EncodeToString(signature)); err == nil {
		t.Fatal("se aceptó una firma alterada")
	}
}
