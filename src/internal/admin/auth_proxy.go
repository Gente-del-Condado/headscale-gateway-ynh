package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// Las identidades de proxy no crean cookies transferibles ni sesiones persistentes.
// Cada petición debe aportar una identidad recién verificada en su contexto confiable.
func (a *AuthManager) proxySession(r *http.Request) (authSession, bool) {
	opts, ok := r.Context().Value(httpRequestOptionsKey{}).(httpRequestOptions)
	if !ok || !opts.trustedProxy || opts.proxyUser == "" {
		return authSession{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.proxyCSRFReady {
		if _, err := rand.Read(a.proxyCSRFKey[:]); err != nil {
			return authSession{}, false
		}
		a.proxyCSRFReady = true
	}
	mac := hmac.New(sha256.New, a.proxyCSRFKey[:])
	mac.Write([]byte(opts.prefix + "\x00" + opts.proxyUser))
	return authSession{user: "proxy:" + opts.proxyUser, role: "admin", source: "proxy",
		csrf: hex.EncodeToString(mac.Sum(nil))}, true
}
