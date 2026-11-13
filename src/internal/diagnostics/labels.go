package diagnostics

import "github.com/gentedelcondado/headscale-gateway/internal/i18n"

// Label traduce las cinco gravedades canónicas; nunca deduce el nivel a partir del mensaje.
func (l Level) Label() string {
	return i18n.T([...]string{"ERROR", "ADVERTENCIA", "INFORMACIÓN", "DEPURACIÓN", "TRAZA"}[l])
}
