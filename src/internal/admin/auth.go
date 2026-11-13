package admin

// Login administrativo: usuarios interactivos usan sesiones HTTP-only.
// El modo de autenticación está centralizado en SQLite.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"golang.org/x/crypto/bcrypt"
)

type LDAPSettings struct {
	URL           string   `json:"url"`
	BaseDN        string   `json:"base_dn"`
	BindDN        string   `json:"bind_dn"`
	BindPassword  string   `json:"bind_password,omitempty"`
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups,omitempty"`
}
type OIDCSettings struct {
	Issuer             string   `json:"issuer"`
	ClientID           string   `json:"client_id"`
	ClientSecret       string   `json:"client_secret,omitempty"`
	RedirectURL        string   `json:"redirect_url"`
	AllowedEmails      []string `json:"allowed_emails"`
	AllowedGroups      []string `json:"allowed_groups,omitempty"`
	DisableGroupsScope bool     `json:"disable_groups_scope,omitempty"`
}
type AuthConfig struct {
	Mode              string            `json:"mode"`
	LocalUsername     string            `json:"local_username"`
	LocalPasswordHash string            `json:"local_password_hash,omitempty"`
	LDAP              LDAPSettings      `json:"ldap"`
	OIDC              OIDCSettings      `json:"oidc"`
	RoleGroups        map[string]string `json:"role_groups,omitempty"`
	RoleEmails        map[string]string `json:"role_emails,omitempty"`
	MFASecret         string            `json:"mfa_secret,omitempty"`
	MFAEnabled        bool              `json:"mfa_enabled,omitempty"`
}
type AuthUpdate struct {
	Mode          string            `json:"mode"`
	LocalUsername string            `json:"local_username"`
	LocalPassword string            `json:"local_password,omitempty"`
	LDAP          LDAPSettings      `json:"ldap"`
	OIDC          OIDCSettings      `json:"oidc"`
	RoleGroups    map[string]string `json:"role_groups,omitempty"`
	RoleEmails    map[string]string `json:"role_emails,omitempty"`
}
type AuthPublic struct {
	Mode             string            `json:"mode"`
	LocalUsername    string            `json:"local_username"`
	HasLocalPassword bool              `json:"has_local_password"`
	LDAP             LDAPSettings      `json:"ldap"`
	HasLDAPPassword  bool              `json:"has_ldap_password"`
	OIDC             OIDCSettings      `json:"oidc"`
	HasOIDCSecret    bool              `json:"has_oidc_secret"`
	RoleGroups       map[string]string `json:"role_groups,omitempty"`
	RoleEmails       map[string]string `json:"role_emails,omitempty"`
	MFAEnabled       bool              `json:"mfa_enabled"`
}
type authSession struct {
	user, csrf, role string
	source           string
	expires          time.Time
	remember         bool
}

// rememberedSession contiene solo usuario, rol, CSRF y expiración; la cookie
// contiene una ID aleatoria y SQLite almacena su hash SHA-256, no la cookie.
// La persistencia se cifra mediante el secretBox del almacén SQLite.
type rememberedSession struct {
	User    string    `json:"user"`
	CSRF    string    `json:"csrf"`
	Role    string    `json:"role"`
	Expires time.Time `json:"expires"`
}
type oidcFlow struct {
	nonce, verifier, state string
	expires                time.Time
}
type loginAttempts struct {
	count int
	start time.Time
}
type AuthManager struct {
	mu                sync.Mutex
	path              string
	store             *state.Store
	config            AuthConfig
	sessions          map[string]authSession
	flows             map[string]oidcFlow
	attempts          map[string]loginAttempts
	bootstrapToken    string
	bootstrapCodeHash [32]byte
	bootstrapUntil    time.Time
	pendingMFA        map[string]pendingTOTP
	usedTOTP          map[string]int64
	proxyCSRFKey      [32]byte
	proxyCSRFReady    bool
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func emptyAuthManager(path string) *AuthManager {
	return &AuthManager{path: path, config: AuthConfig{Mode: "token", LocalUsername: "admin"}, sessions: map[string]authSession{}, flows: map[string]oidcFlow{}, attempts: map[string]loginAttempts{}, pendingMFA: map[string]pendingTOTP{}, usedTOTP: map[string]int64{}}
}
func NewAuthManager(path string) (*AuthManager, error) {
	a := emptyAuthManager(path)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (os.PathSeparator == '/' && info.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("auth.json requiere permisos 0600 y archivo regular")
	}
	if err = json.Unmarshal(b, &a.config); err != nil {
		return nil, fmt.Errorf("auth.json inválido: %w", err)
	}
	if err = validateAuthConfig(a.config); err != nil {
		return nil, err
	}
	return a, nil
}

// UseStore migra auth.json a SQLite en el primer uso. Los cambios posteriores
// usan SQLite; el archivo antiguo queda intacto como copia privada para revertir.
// Nunca elimina la base de datos sin exportar antes la configuración actual.
func (a *AuthManager) UseStore(db *state.Store) error {
	if db == nil {
		return errors.New("SQLite nulo")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var restored AuthConfig
	exists, err := db.GetJSON("auth_config_v1", &restored)
	if err != nil {
		return err
	}
	if exists {
		if err = validateAuthConfig(restored); err != nil {
			return err
		}
		a.config = restored
	} else if err = db.PutJSON("auth_config_v1", a.config); err != nil {
		return err
	}
	a.store = db
	return a.loadRememberedSessionsLocked()
}

// NewAuthManagerWithStore prefers SQLite over any stale legacy auth.json.
// On first migration, it reads auth.json and writes an exact copy to SQLite.
func NewAuthManagerWithStore(path string, db *state.Store) (*AuthManager, error) {
	if db == nil {
		return nil, errors.New("SQLite requerido")
	}
	var existing AuthConfig
	found, err := db.GetJSON("auth_config_v1", &existing)
	if err != nil {
		return nil, err
	}
	if found {
		if err = validateAuthConfig(existing); err != nil {
			return nil, err
		}
		a := emptyAuthManager(path)
		a.config = existing
		a.store = db
		if err := a.loadRememberedSessionsLocked(); err != nil {
			return nil, err
		}
		return a, nil
	}
	a, err := NewAuthManager(path)
	if err != nil {
		return nil, err
	}
	if err = a.UseStore(db); err != nil {
		return nil, err
	}
	return a, nil
}

// NewAuthManagerSQLite nunca lee auth.json. Los ajustes de identidad se guardan
// exclusivamente en la base de datos privada del Gateway.
func NewAuthManagerSQLite(db *state.Store) (*AuthManager, error) {
	if db == nil {
		return nil, errors.New("SQLite requerido")
	}
	a := emptyAuthManager("")
	if err := a.UseStore(db); err != nil {
		return nil, err
	}
	return a, nil
}

func validateAuthConfig(c AuthConfig) error {
	switch c.Mode {
	case "token":
		return nil
	case "local":
		if c.LocalUsername == "" || c.LocalPasswordHash == "" {
			return errors.New("usuario y contraseña local obligatorios")
		}
	case "ldap":
		u, e := url.Parse(c.LDAP.URL)
		if e != nil || u.Scheme != "ldaps" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("LDAP requiere ldaps:// con certificado confiable")
		}
		if c.LDAP.BaseDN == "" || (len(c.LDAP.AllowedUsers) == 0 && len(c.LDAP.AllowedGroups) == 0) {
			return errors.New("LDAP requiere base_dn y allowed_users")
		}
	case "oidc":
		u, e := url.Parse(c.OIDC.Issuer)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("OIDC requiere issuer HTTPS")
		}
		cb, e := url.Parse(c.OIDC.RedirectURL)
		prefix := ""
		if cb != nil {
			prefix = strings.TrimSuffix(cb.Path, "/admin/auth/oidc/callback")
		}
		if e != nil || cb.Scheme != "https" || cb.Host == "" || !strings.HasSuffix(cb.Path, "/admin/auth/oidc/callback") || !basePathPattern.MatchString(prefix) || cb.User != nil || cb.RawQuery != "" || cb.Fragment != "" {
			return errors.New("OIDC requiere redirect_url HTTPS terminado en /admin/auth/oidc/callback")
		}
		if c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "" || (len(c.OIDC.AllowedEmails) == 0 && len(c.OIDC.AllowedGroups) == 0) {
			return errors.New("OIDC requiere client_id, secreto y al menos un email autorizado")
		}
	default:
		return errors.New("modo de autenticación no soportado")
	}
	if c.Mode == "ldap" && len(c.RoleGroups) == 0 {
		return errors.New("LDAP requiere mapeo explícito de grupos a roles")
	}
	if c.Mode == "oidc" && len(c.RoleGroups) == 0 && len(c.RoleEmails) == 0 {
		return errors.New("OIDC requiere mapeo explícito de grupos o correos a roles")
	}
	for _, role := range c.RoleGroups {
		if role != "admin" && role != "operator" && role != "viewer" {
			return errors.New("role_groups contiene un rol desconocido")
		}
	}
	for email, role := range c.RoleEmails {
		if strings.TrimSpace(email) != email || strings.ContainsAny(email, " \t\r\n") || len(strings.Split(email, "@")) != 2 || !strings.Contains(strings.Split(email, "@")[1], ".") || role != "admin" && role != "operator" && role != "viewer" {
			return errors.New("role_emails contiene correo o rol no válido")
		}
		if c.Mode == "oidc" && !containsIdentity(c.OIDC.AllowedEmails, email) {
			return errors.New("cada correo con rol OIDC debe aparecer en correos autorizados")
		}
	}
	if c.Mode == "oidc" && c.OIDC.DisableGroupsScope && (len(c.RoleEmails) == 0 || len(c.OIDC.AllowedGroups) > 0) {
		return errors.New("OIDC sin scope groups requiere rol por correo y grupos admitidos vacíos")
	}
	if c.Mode == "oidc" && len(c.RoleEmails) > 0 && len(c.OIDC.AllowedEmails) == 0 {
		return errors.New("OIDC por correo requiere correos autorizados explícitos")
	}
	if c.MFAEnabled && c.MFASecret == "" {
		return errors.New("MFA habilitado sin secreto")
	}
	return nil
}
func (a *AuthManager) Current() AuthConfig { a.mu.Lock(); defer a.mu.Unlock(); return a.config }
func (a *AuthManager) Public() AuthPublic {
	c := a.Current()
	c.LDAP.BindPassword = ""
	c.OIDC.ClientSecret = ""
	return AuthPublic{Mode: c.Mode, LocalUsername: c.LocalUsername, HasLocalPassword: c.LocalPasswordHash != "", LDAP: c.LDAP, HasLDAPPassword: a.hasLDAPPassword(), OIDC: c.OIDC, HasOIDCSecret: a.hasOIDCSecret(), RoleGroups: c.RoleGroups, RoleEmails: c.RoleEmails, MFAEnabled: c.MFAEnabled}
}
func (a *AuthManager) hasLDAPPassword() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.LDAP.BindPassword != ""
}
func (a *AuthManager) hasOIDCSecret() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config.OIDC.ClientSecret != ""
}
func (a *AuthManager) Update(u AuthUpdate) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Mode = u.Mode
	if u.LocalUsername != "" {
		c.LocalUsername = strings.TrimSpace(u.LocalUsername)
	}
	if u.LocalPassword != "" {
		if len(u.LocalPassword) < 12 || len(u.LocalPassword) > 72 {
			return errors.New("contraseña local: entre 12 y 72 bytes")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(u.LocalPassword), bcrypt.DefaultCost)
		if e != nil {
			return e
		}
		c.LocalPasswordHash = string(hash)
	}
	if u.LDAP.BindPassword == "" {
		u.LDAP.BindPassword = c.LDAP.BindPassword
	}
	if u.OIDC.ClientSecret == "" {
		u.OIDC.ClientSecret = c.OIDC.ClientSecret
	}
	c.LDAP = u.LDAP
	c.OIDC = u.OIDC
	c.RoleGroups = u.RoleGroups
	c.RoleEmails = u.RoleEmails
	if err := validateAuthConfig(c); err != nil {
		return err
	}
	if a.store != nil {
		if err := a.store.PutJSON("auth_config_v1", c); err != nil {
			return err
		}
		a.config = c
		a.sessions = map[string]authSession{}
		return a.saveRememberedSessionsLocked()
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	name, e := randomHex(10)
	if e != nil {
		return e
	}
	tmp := filepath.Join(filepath.Dir(a.path), ".auth-"+name)
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, a.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	a.config = c
	// Revocar sesiones previas al cambiar de modo o credenciales.
	a.sessions = map[string]authSession{}
	return a.saveRememberedSessionsLocked()
}
func (a *AuthManager) Methods() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{"mode": a.config.Mode, "mfa_enabled": a.config.Mode == "local" && a.config.MFAEnabled, "token_recovery": true, "bootstrap": a.bootstrapToken != "" && time.Now().Before(a.bootstrapUntil)}
}
func (a *AuthManager) SetBootstrap(token, code string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bootstrapToken = token
	a.bootstrapCodeHash = sha256.Sum256([]byte(code))
	a.bootstrapUntil = time.Now().Add(15 * time.Minute)
}
func (a *AuthManager) ClearBootstrap() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bootstrapToken = ""
	a.bootstrapCodeHash = [32]byte{}
	a.bootstrapUntil = time.Time{}
}
func loopbackRequest(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func (a *AuthManager) RedeemBootstrap(code string, r *http.Request) (string, error) {
	if !loopbackRequest(r) {
		opts, ok := r.Context().Value(httpRequestOptionsKey{}).(httpRequestOptions)
		if !ok || !opts.trustedProxy {
			return "", errors.New("bootstrap requiere acceso local o proxy autorizado")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bootstrapToken == "" || time.Now().After(a.bootstrapUntil) {
		return "", errors.New("código de inicio caducado o utilizado")
	}
	got := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(got[:], a.bootstrapCodeHash[:]) != 1 {
		return "", errors.New("código de inicio incorrecto")
	}
	token := a.bootstrapToken
	a.bootstrapToken = ""
	a.bootstrapCodeHash = [32]byte{}
	return token, nil
}
func (a *AuthManager) TooManyAttempts(addr string) bool {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	v := a.attempts[addr]
	if now.Sub(v.start) > 5*time.Minute {
		v = loginAttempts{start: now}
	}
	v.count++
	a.attempts[addr] = v
	return v.count > 8
}
func (a *AuthManager) VerifyLocal(user, pass string) bool {
	c := a.Current()
	if c.Mode != "local" || subtle.ConstantTimeCompare([]byte(user), []byte(c.LocalUsername)) != 1 {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(c.LocalPasswordHash), []byte(pass)) == nil
}
func containsIdentity(allowed []string, actual string) bool {
	for _, v := range allowed {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(actual)) {
			return true
		}
	}
	return false
}

// sessionKey es el resumen SHA-256 de una cookie opaca de 256 bits.
func sessionKey(cookie string) string {
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:])
}

// loadRememberedSessionsLocked se llama manteniendo mu bloqueado durante UseStore.
func (a *AuthManager) loadRememberedSessionsLocked() error {
	if a.store == nil {
		return nil
	}
	var saved map[string]rememberedSession
	found, err := a.store.GetJSON("auth.remembered_sessions.v1", &saved)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	for key, s := range saved {
		if time.Now().Before(s.Expires) && s.User != "" && s.CSRF != "" && s.Role != "" {
			a.sessions[key] = authSession{user: s.User, csrf: s.CSRF, role: s.Role, expires: s.Expires, remember: true}
		}
	}
	return nil
}

func (a *AuthManager) saveRememberedSessionsLocked() error {
	if a.store == nil {
		return nil
	}
	saved := make(map[string]rememberedSession)
	for id, s := range a.sessions {
		if s.remember && time.Now().Before(s.expires) {
			saved[id] = rememberedSession{User: s.user, CSRF: s.csrf, Role: s.role, Expires: s.expires}
		}
	}
	return a.store.PutJSON("auth.remembered_sessions.v1", saved)
}

func (a *AuthManager) CreateSession(w http.ResponseWriter, r *http.Request, user string) string {
	return a.CreateSessionRole(w, r, user, "admin")
}
func (a *AuthManager) CreateSessionRole(w http.ResponseWriter, r *http.Request, user, role string) string {
	csrf, _ := a.CreateSessionRoleRemember(w, r, user, role, false)
	return csrf
}

// CreateSessionRoleRemember permite recordar voluntariamente una sesión web.
// Nunca guarda contraseñas ni tokens de recuperación en cookies o localStorage.
func (a *AuthManager) CreateSessionRoleRemember(w http.ResponseWriter, r *http.Request, user, role string, remember bool) (string, error) {
	id, err := randomHex(32)
	if err != nil {
		return "", err
	}
	csrf, err := randomHex(32)
	if err != nil {
		return "", err
	}
	duration := 8 * time.Hour
	if remember {
		duration = 30 * 24 * time.Hour
	}
	key := sessionKey(id)
	a.mu.Lock()
	a.sessions[key] = authSession{user: user, csrf: csrf, role: role, expires: time.Now().Add(duration), remember: remember}
	if remember {
		if err := a.saveRememberedSessionsLocked(); err != nil {
			delete(a.sessions, key)
			a.mu.Unlock()
			return "", err
		}
	}
	secure := r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://") || a.config.Mode == "oidc"
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "hg_admin_session", Value: id, Path: externalPath(r, "/"), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: int(duration.Seconds())})
	return csrf, nil
}
func (a *AuthManager) Session(r *http.Request) (authSession, bool) {
	if session, ok := a.proxySession(r); ok {
		return session, true
	}
	cookie, err := r.Cookie("hg_admin_session")
	if err != nil || len(cookie.Value) != 64 {
		return authSession{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := sessionKey(cookie.Value)
	s, ok := a.sessions[key]
	if !ok {
		return authSession{}, false
	}
	if time.Now().After(s.expires) {
		delete(a.sessions, key)
		if s.remember {
			_ = a.saveRememberedSessionsLocked()
		}
		return authSession{}, false
	}
	return s, true
}
func (a *AuthManager) Logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("hg_admin_session"); e == nil {
		a.mu.Lock()
		key := sessionKey(c.Value)
		remember := a.sessions[key].remember
		delete(a.sessions, key)
		if remember {
			_ = a.saveRememberedSessionsLocked()
		}
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "hg_admin_session", Value: "", Path: externalPath(r, "/"), HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (a *AuthManager) sessionAllowed(r *http.Request) bool {
	s, ok := a.Session(r)
	if !ok {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	supplied := r.Header.Get("X-CSRF-Token")
	return supplied != "" && subtle.ConstantTimeCompare([]byte(s.csrf), []byte(supplied)) == 1
}
func (a *AuthManager) userSession(r *http.Request) map[string]any {
	s, ok := a.Session(r)
	if !ok {
		return map[string]any{"authenticated": false}
	}
	return map[string]any{"authenticated": true, "user": s.user, "csrf": s.csrf, "role": s.role, "source": s.source}
}
