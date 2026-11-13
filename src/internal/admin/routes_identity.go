package admin

// Identity endpoints: login, OIDC, password and second-factor enrollment.
import "net/http"

func (s *Server) registerAuthRoutes(mux routeRegistrar) {
	// Autenticación interactiva: las rutas de recuperación siguen utilizando el token anterior.
	mux.HandleFunc("GET /api/v1/auth/methods", func(w http.ResponseWriter, r *http.Request) {
		if opts, ok := r.Context().Value(httpRequestOptionsKey{}).(httpRequestOptions); ok && opts.proxyUser != "" {
			reply(w, 200, map[string]any{"mode": "proxy", "bootstrap": false, "token_recovery": false}, nil)
			return
		}
		if s.auth == nil {
			reply(w, 200, map[string]any{"mode": "token", "token_recovery": true}, nil)
			return
		}
		reply(w, 200, s.auth.Methods(), nil)
	})
	mux.HandleFunc("POST /api/v1/auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 404, "bootstrap no disponible")
			return
		}
		var v struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &v) {
			return
		}
		if s.auth.TooManyAttempts(r.RemoteAddr) {
			problem(w, 429, "demasiados intentos")
			return
		}
		token, err := s.auth.RedeemBootstrap(v.Code, r)
		if err != nil {
			problem(w, 403, err.Error())
			return
		}
		reply(w, 200, map[string]string{"token": token}, nil)
	})
	mux.HandleFunc("GET /api/v1/auth/session", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			reply(w, 200, map[string]any{"authenticated": true, "user": "token"}, nil)
			return
		}
		if _, ok := s.auth.Session(r); ok {
			reply(w, 200, s.auth.userSession(r), nil)
		} else {
			reply(w, 200, map[string]any{"authenticated": true, "user": "token"}, nil)
		}
	}))
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 404, "acceso interactivo no disponible")
			return
		}
		if !sameOrigin(r) {
			problem(w, 403, "origen inválido")
			return
		}
		if s.auth.TooManyAttempts(r.RemoteAddr) {
			problem(w, 429, "demasiados intentos; prueba más tarde")
			return
		}
		var v struct {
			Username string `json:"username"`
			Password string `json:"password"`
			TOTP     string `json:"totp"`
			Remember bool   `json:"remember"`
		}
		if !decode(w, r, &v) {
			return
		}
		c := s.auth.Current()
		var ok bool
		switch c.Mode {
		case "local":
			ok = s.auth.VerifyLocal(v.Username, v.Password)
		case "ldap":
			ok = v.Username != "" && v.Password != "" // LDAP verification follows in role assignment
		default:
			problem(w, 400, "utiliza el token o el proveedor OIDC")
			return
		}
		if ok && c.Mode == "local" && !s.auth.VerifyLocalMFA(v.TOTP) {
			ok = false
		}
		if !ok {
			problem(w, 401, "credenciales incorrectas o segundo factor inválido")
			return
		}
		role := "admin"
		if c.Mode == "ldap" {
			groups, err := LDAPGroups(c.LDAP, v.Username, v.Password)
			if err != nil {
				problem(w, 401, "no se pudieron verificar grupos LDAP")
				return
			}
			role = roleForGroups(c.RoleGroups, groups, c.LDAP.AllowedGroups)
		}
		if role == "" {
			problem(w, 403, "sin permiso asignado")
			return
		}
		csrf, err := s.auth.CreateSessionRoleRemember(w, r, v.Username, role, v.Remember)
		if err != nil {
			problem(w, 503, "no se pudo guardar la sesión")
			return
		}
		reply(w, 200, map[string]any{"authenticated": true, "user": v.Username, "csrf": csrf}, nil)
	})
	mux.HandleFunc("POST /api/v1/auth/logout", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.auth != nil {
			if session, ok := s.auth.Session(r); ok && session.source == "proxy" {
				problem(w, 403, "La sesión se gestiona desde el sistema anfitrión.")
				return
			}
			s.auth.Logout(w, r)
		}
		reply(w, 200, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/auth/settings", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 501, "autenticación no configurada")
			return
		}
		reply(w, 200, s.auth.Public(), nil)
	}))
	mux.HandleFunc("PUT /api/v1/auth/settings", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 501, "autenticación no configurada")
			return
		}
		var update AuthUpdate
		if !decode(w, r, &update) {
			return
		}
		if err := s.auth.Update(update); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"ok": true, "reauthentication_required": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/auth/mfa/setup", s.secure(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.auth.Session(r)
		if !ok || sess.source == "proxy" {
			problem(w, 403, "MFA requiere sesión interactiva local")
			return
		}
		secret, uri, err := s.auth.MFASetup(sess.user)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]string{"secret": secret, "otpauth_url": uri}, nil)
	}))
	mux.HandleFunc("POST /api/v1/auth/mfa/confirm", s.secure(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.auth.Session(r)
		if !ok || sess.source == "proxy" {
			problem(w, 403, "sesión local requerida")
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &req) {
			return
		}
		if err := s.auth.MFAConfirm(sess.user, req.Code); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"enabled": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/auth/mfa/disable", s.secure(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.auth.Session(r)
		if !ok || sess.source == "proxy" {
			problem(w, 403, "sesión local requerida")
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &req) {
			return
		}
		if err := s.auth.MFADisable(sess.user, req.Code); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"enabled": false}, nil)
	}))

	// Solo una sesión administrativa real (OIDC/local/LDAP y CSRF) puede rotar
	// la credencial de emergencia. Una clave Bearer filtrada no puede rotarse a sí misma.
	mux.HandleFunc("POST /api/v1/auth/recovery/rotate", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) || s.auth == nil {
			problem(w, 403, "se requiere una sesión administrativa interactiva")
			return
		}
		if _, ok := s.auth.Session(r); !ok {
			problem(w, 403, "se requiere una sesión administrativa interactiva")
			return
		}
		var req struct {
			Confirm string `json:"confirm"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Confirm != "ROTAR" {
			problem(w, 400, "escribe ROTAR para confirmar")
			return
		}
		secret, err := s.rotateRecoveryToken()
		if err != nil {
			problem(w, 503, "no se pudo rotar la credencial")
			return
		}
		// El secreto completo solo se entrega en esta respuesta autenticada con no-store.
		reply(w, 200, map[string]string{"token": secret}, nil)
	}))
	mux.HandleFunc("GET /admin/auth/oidc/start", func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 404, "OIDC no disponible")
			return
		}
		s.beginOIDC(w, r)
	})
	mux.HandleFunc("GET /admin/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			problem(w, 404, "OIDC no disponible")
			return
		}
		s.finishOIDC(w, r)
	})

}
