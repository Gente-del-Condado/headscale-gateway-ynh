package admin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func oidcClient(ctx context.Context, c OIDCSettings) (*oidc.Provider, *oauth2.Config, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return nil, nil, errors.New("no se pudo descubrir el proveedor OIDC")
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if !c.DisableGroupsScope {
		scopes = append(scopes, "groups")
	}
	return provider, &oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: c.RedirectURL, Scopes: scopes}, nil
}
func (s *Server) beginOIDC(w http.ResponseWriter, r *http.Request) {
	c := s.auth.Current()
	if c.Mode != "oidc" {
		problem(w, 400, "OIDC no está activo")
		return
	}
	provider, oauth, err := oidcClient(r.Context(), c.OIDC)
	_ = provider
	if err != nil {
		problem(w, 502, err.Error())
		return
	}
	state, err := randomHex(24)
	if err != nil {
		problem(w, 500, "no se pudo crear estado aleatorio")
		return
	}
	nonce, err := randomHex(24)
	if err != nil {
		problem(w, 500, "no se pudo crear nonce aleatorio")
		return
	}
	verifier := oauth2.GenerateVerifier()
	s.auth.mu.Lock()
	// Recortar flujos caducados y limitar las solicitudes pendientes.
	for k, v := range s.auth.flows {
		if time.Now().After(v.expires) {
			delete(s.auth.flows, k)
		}
	}
	if len(s.auth.flows) > 250 {
		s.auth.mu.Unlock()
		problem(w, 429, "demasiados inicios pendientes")
		return
	}
	s.auth.flows[state] = oidcFlow{nonce: nonce, verifier: verifier, state: state, expires: time.Now().Add(10 * time.Minute)}
	s.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "hg_oidc_state", Value: state, Path: externalPath(r, "/admin/auth/oidc/callback"), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, oauth.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}
func (s *Server) finishOIDC(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("hg_oidc_state")
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || cookie.Value != state {
		http.Error(w, "Estado OIDC inválido", 400)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "hg_oidc_state", Value: "", Path: externalPath(r, "/admin/auth/oidc/callback"), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	s.auth.mu.Lock()
	flow, ok := s.auth.flows[state]
	delete(s.auth.flows, state)
	s.auth.mu.Unlock()
	if !ok || time.Now().After(flow.expires) || r.URL.Query().Get("code") == "" {
		http.Error(w, "Autenticación OIDC caducada", 400)
		return
	}
	c := s.auth.Current()
	if c.Mode != "oidc" {
		http.Error(w, "OIDC desactivado", 403)
		return
	}
	provider, oauth, err := oidcClient(r.Context(), c.OIDC)
	if err != nil {
		http.Error(w, "Proveedor OIDC no disponible", 502)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	tok, err := oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(flow.verifier))
	if err != nil {
		http.Error(w, "Intercambio OIDC fallido", 401)
		return
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Falta ID token", 401)
		return
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: c.OIDC.ClientID}).Verify(ctx, raw)
	if err != nil {
		http.Error(w, "ID token inválido", 401)
		return
	}
	var claims struct {
		Nonce         string   `json:"nonce"`
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Groups        []string `json:"groups"`
	}
	if err = verified.Claims(&claims); err != nil || claims.Nonce != flow.nonce || !claims.EmailVerified || (len(c.OIDC.AllowedEmails) > 0 && !containsIdentity(c.OIDC.AllowedEmails, claims.Email)) {
		http.Error(w, "Usuario OIDC no autorizado", 403)
		return
	}
	if len(c.OIDC.AllowedGroups) > 0 {
		any := false
		for _, g := range claims.Groups {
			if containsIdentity(c.OIDC.AllowedGroups, g) {
				any = true
				break
			}
		}
		if !any {
			http.Error(w, "Grupo OIDC no autorizado", 403)
			return
		}
	}
	role := roleForOIDC(c.RoleGroups, c.RoleEmails, claims.Groups, c.OIDC.AllowedGroups, claims.Email)
	if role == "" {
		http.Error(w, "Grupo sin rol autorizado", 403)
		return
	}
	s.auth.CreateSessionRole(w, r, claims.Email, role)
	http.Redirect(w, r, externalPath(r, "/admin/"), http.StatusSeeOther)
}

// Prepara un hash de diagnóstico para la documentación de PKCE. El valor real
// de code_verifier permanece solo en memoria y nunca se envía al panel.
func oidcChallenge(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func validateOIDCRedirect(v string) bool {
	u, e := url.Parse(v)
	return e == nil && u.Scheme == "https" && strings.HasSuffix(u.Path, "/admin/auth/oidc/callback")
}
