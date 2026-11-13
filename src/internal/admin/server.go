// Package admin ofrece una API REST y un panel de control sin dependencias externas.
// El listener se configura en main, separado del proxy HTTPS del Gateway.
package admin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

// Service conserva el proxy avanzado como JSON para permitir editarlo sin perder opciones.
type Service struct {
	Description string          `json:"description,omitempty"`
	Name        string          `json:"name"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Backend     string          `json:"backend"`
	Template    string          `json:"template,omitempty"`
	Proxy       json.RawMessage `json:"proxy,omitempty"`
}

// ConfigOption es una opción válida de un campo de configuración.
// Compartido entre el esquema REST, la validación del Gateway y el panel.
type ConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ConfigField describe un campo editable de configuración SQLite.
// Se mantiene en el mismo archivo que Controller para evitar dependencias
// accidentales de archivos auxiliares al integrar el servidor admin.
type ConfigField struct {
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Group       string              `json:"group"`
	Kind        string              `json:"kind"`
	Description string              `json:"description,omitempty"`
	Options     []ConfigOption      `json:"options,omitempty"`
	Required    bool                `json:"required,omitempty"`
	VisibleWhen map[string][]string `json:"visible_when,omitempty"`
}

// Controller mantiene la lógica de acceso a disco, validación y publicación fuera de HTTP.
type Controller interface {
	Status(context.Context) (any, error)
	Services(context.Context) ([]Service, error)
	Templates(context.Context) ([]string, error)
	Settings(context.Context) (any, error)
	ConfigSchema(context.Context) ([]ConfigField, error)
	UpdateSettings(context.Context, map[string]string) error
	SaveService(context.Context, Service) error
	DeleteService(context.Context, string) error
	Reload(context.Context) error
}

type RestartController interface {
	ScheduleRestart(context.Context) error
}

type NetworkController interface {
	NetworkInfo(context.Context) (any, error)
	NetworkNodes(context.Context) (any, error)
	NetworkPing(context.Context, string, string) (any, error)
	NetworkLogin(context.Context) (any, error)
}

var ErrNetworkDisabled = errors.New("panel de red desactivado")
var ErrAlreadyReplied = errors.New("respuesta enviada")

type adminRoleKey struct{}

func (s *Server) isAdminRequest(r *http.Request) bool {
	return r.Context().Value(adminRoleKey{}) == "admin"
}

type Server struct {
	controller   Controller
	tokenHash    atomic.Pointer[[32]byte]
	recoveryMu   sync.Mutex
	auth         *AuthManager
	policy       *APIPolicyManager
	database     *state.Store
	requestCount atomic.Uint64
	backups      *backupManager
}

func New(token string, controller Controller) (*Server, error) {
	if len(token) < 32 || controller == nil {
		return nil, errors.New("token de administración insuficiente o controlador ausente")
	}
	s := &Server{controller: controller}
	h := sha256.Sum256([]byte(token))
	s.tokenHash.Store(&h)
	return s, nil
}

func (s *Server) WithAuth(auth *AuthManager) *Server { s.auth = auth; return s }

// WithState activa política y auditoría sobre SQLite. Si la base de datos no
// se inicializa, el arranque falla; nunca continúa silenciosamente sin controles.
func (s *Server) WithState(db *state.Store) error {
	policy, err := NewAPIPolicyManager(db)
	if err != nil {
		return err
	}
	s.database = db
	s.policy = policy
	return s.initBackups()
}

// Handler monta la interfaz y los endpoints una sola vez. Las cabeceras,
// trazabilidad y seguridad HTTP se aplican a ambos tipos de rutas.
func (s *Server) Handler(options ...HTTPOptions) http.Handler {
	mux := &referenceMux{ServeMux: http.NewServeMux()}
	s.registerStaticRoutes(mux)
	s.registerAPIRoutes(mux)
	s.registerReferenceRoutes(mux)
	var opts HTTPOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return opts.wrap(s.wrapHTTP(mux.ServeMux))
}

// recoveryTokenMatches comprueba la credencial de recuperación sin asumir
// que el servidor fue construido mediante New. Un servidor incompleto (por
// ejemplo, creado por una prueba) nunca debe autenticar tokens ni provocar
// un panic por la ausencia del hash.
func (s *Server) recoveryTokenMatches(header string) bool {
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	stored := s.tokenHash.Load()
	if stored == nil {
		return false
	}
	return subtle.ConstantTimeCompare(stored[:], hash(strings.TrimPrefix(header, "Bearer "))) == 1
}

// secure aplica autenticación y autorización por roles. El token de
// recuperación tiene rol admin, sujeto a la política API persistida. No se
// considera RemoteAddr==loopback una prueba de identidad: un proxy inverso
// podría reenviar solicitudes externas desde la propia máquina.
func (s *Server) secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, role, tokenAuth := "", "", false
		header := r.Header.Get("Authorization")
		if s.recoveryTokenMatches(header) {
			actor, role, tokenAuth = "recovery-token", "admin", true
		} else if s.database != nil && strings.HasPrefix(header, "Bearer hgk_") {
			info, ok, err := s.database.ResolveToken(strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				problem(w, 503, "Almacenamiento API no disponible")
				return
			}
			if !ok {
				problem(w, 401, "token no válido o caducado")
				return
			}
			role, actor, tokenAuth = info.Role, "api:"+info.ID, true
			// Los alcances del token siempre limitan el rol; no se confía en alcances del cliente.
			allowed := false
			for _, scope := range info.Scopes {
				if scope == routeCategory(r) {
					allowed = true
					break
				}
			}
			if !allowed {
				problem(w, 403, "el token no tiene permiso para el módulo")
				return
			}
		} else if s.auth != nil && s.auth.sessionAllowed(r) {
			sess, _ := s.auth.Session(r)
			actor, role = sess.user, sess.role
			// La ausencia de rol nunca concede permisos: autenticar no equivale a autorizar.
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			problem(w, 401, "sesión, CSRF o token no válidos")
			return
		}
		category := routeCategory(r)
		r = r.WithContext(context.WithValue(r.Context(), adminRoleKey{}, role))
		if r.URL.Path == "/api/v1/auth/session" && tokenAuth {
			reply(w, 200, map[string]any{"authenticated": true, "user": actor, "role": role}, nil)
			return
		}
		if !rolePermits(role, category, r.Method) {
			problem(w, 403, "permisos insuficientes para esta operación")
			return
		}
		if tokenAuth && s.policy != nil && !s.policy.TokenAllowed(category) {
			problem(w, 403, "API Bearer deshabilitada para este módulo")
			return
		}
		// Audita solo cambios de estado, sin registrar secretos ni cuerpos de petición.
		if s.database != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			recorder := &statusWriter{ResponseWriter: w, status: 200}
			next(recorder, r)
			outcome := "ok"
			if recorder.status >= 400 {
				outcome = "denied-or-error"
			}
			_ = s.database.Audit(actor, category+"."+strings.ToLower(r.Method), outcome)
			diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-AUDIT-001", "actor=%q area=%q action=%q outcome=%q", actor, category, r.Method, outcome)
			return
		}
		next(w, r)
	}
}

type languageWriter struct {
	http.ResponseWriter
	language string
	status   int
}

func (w *languageWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *languageWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *languageWriter) Language() string { return w.language }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Language() string {
	if l, ok := w.ResponseWriter.(interface{ Language() string }); ok {
		return l.Language()
	}
	return "es"
}
func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func hash(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }
func (s *Server) read(w http.ResponseWriter, r *http.Request, fn func(context.Context) (any, error)) {
	v, e := fn(r.Context())
	reply(w, http.StatusOK, v, e)
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		problem(w, 415, "Content-Type debe ser application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if e := dec.Decode(dst); e != nil {
		problem(w, 400, fmt.Sprintf("JSON inválido: %v", e))
		return false
	}
	var tail any
	if e := dec.Decode(&tail); e != io.EOF {
		problem(w, 400, "solo se admite un objeto JSON")
		return false
	}
	return true
}
func reply(w http.ResponseWriter, status int, v any, e error) {
	if e != nil {
		problem(w, 500, "operación fallida")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	lang := "es"
	if lw, ok := w.(interface{ Language() string }); ok {
		lang = lw.Language()
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"error": i18n.Translate(lang, msg)})
}

// Los formularios interactivos requieren origen coincidente. El proxy debe
// conservar Host; nunca confiar en X-Forwarded-Host enviado por clientes.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host) && (u.Scheme == "http" || u.Scheme == "https")
}

// rotateRecoveryToken guarda y activa un token de emergencia sin reiniciar.
// Invalida el anterior inmediatamente. El llamante debe ser una sesión de
// administrador autenticada; este método no autoriza peticiones.
func (s *Server) rotateRecoveryToken() (string, error) {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if s.database == nil {
		return "", errors.New("SQLite no disponible")
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	// Borra la inicialización pendiente antes de cambiar el token activo.
	// Si falla el almacenamiento, el hash anterior sigue funcionando en este proceso.
	if err = s.database.PutJSON("recovery_bootstrap_pending_v1", false); err != nil {
		return "", err
	}
	if err = s.database.PutJSON("recovery_token_v1", secret); err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(secret))
	s.tokenHash.Store(&h)
	if s.auth != nil {
		s.auth.ClearBootstrap()
	}
	return secret, nil
}
