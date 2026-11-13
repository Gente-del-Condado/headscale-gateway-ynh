// El paquete policytemplates gestiona ejemplos ACL Headscale portables, no ejecutables.
// Son borradores y nunca se envían automáticamente al servidor Headscale.
package policytemplates

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tailscale/hujson"
	"regexp"
	"strings"
)

const Format = "headscale-gateway-acl-templates"
const MaxTemplates = 64
const MaxStoredTemplates = 20
const MaxStoredBytes = 14 * 1024
const MaxCatalogBytes = 64 * 1024

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Policy      string `json:"policy"`
	Source      string `json:"source,omitempty"`
}
type Bundle struct {
	Format    string     `json:"format"`
	Version   int        `json:"version"`
	Templates []Template `json:"templates"`
}

func Normalize(raw string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > 100*1024 {
		return nil, errors.New("política vacía o demasiado grande")
	}
	data, err := hujson.Standardize([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("huJSON inválido: %w", err)
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, errors.New("la política debe ser un objeto huJSON")
	}
	return object, nil
}

func Validate(t Template) error {
	if !idPattern.MatchString(t.ID) || len(t.Name) < 2 || len(t.Name) > 100 || len(t.Description) > 800 || len(t.Warning) > 300 || len(t.Category) > 60 || strings.TrimSpace(t.Policy) == "" || len(t.Policy) > 16*1024 {
		return errors.New("identificador, nombre o tamaño de plantilla inválido")
	}
	obj, err := Normalize(t.Policy)
	if err != nil {
		return err
	}
	// Los esquemas visuales son HuJSON Headscale; impide aplicar metadatos arbitrarios sin aviso.
	for key := range obj {
		if len(key) > 128 {
			return errors.New("clave de política demasiado larga")
		}
	}
	return nil
}
func Decode(data []byte) (Bundle, error) {
	var b Bundle
	if len(data) > MaxCatalogBytes {
		return b, errors.New("repositorio de políticas demasiado grande (máximo 64 KiB)")
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, err
	}
	if b.Format != Format || b.Version != 1 || len(b.Templates) == 0 || len(b.Templates) > MaxTemplates {
		return b, errors.New("formato o versión del repositorio no admitidos")
	}
	seen := map[string]bool{}
	for _, v := range b.Templates {
		if err := Validate(v); err != nil {
			return b, fmt.Errorf("%s: %w", v.ID, err)
		}
		if seen[v.ID] {
			return b, errors.New("IDs duplicados")
		}
		seen[v.ID] = true
	}
	return b, nil
}
func Builtins() []Template {
	return append([]Template{
		{ID: "acceso-libre", Name: "Mantener acceso entre todos los equipos", Category: "Inicio", Description: "Política vacía. No cambia el comportamiento abierto predeterminado de Headscale.", Warning: "Permite todo el tráfico entre nodos; no aísla usuarios.", Policy: "{}", Source: "builtin"},
		{ID: "bloqueo-total", Name: "Bloquear todas las conexiones", Category: "Avanzada", Description: "Política restrictiva inicial para construir autorizaciones desde cero.", Warning: "ATENCIÓN: puede cortar todas las conexiones de la tailnet, incluido el acceso administrativo remoto.", Policy: "{\"grants\":[]}", Source: "builtin"},
		{ID: "equipos-propios", Name: "Cada usuario accede a sus propios equipos", Category: "Privacidad", Description: "Ejemplo con autogroup:member y autogroup:self; comprueba su compatibilidad con tus nodos.", Warning: "Los nodos con etiquetas y ciertos escenarios compartidos necesitan permisos adicionales.", Policy: "{\"grants\":[{\"src\":[\"autogroup:member\"],\"dst\":[\"autogroup:self\"],\"ip\":[\"*\"]}]}", Source: "builtin"},
		{ID: "web-interna", Name: "Permitir únicamente HTTP y HTTPS", Category: "Servicios", Description: "Ejemplo de Grant que permite puertos 80 y 443 entre miembros.", Warning: "Bloquea el resto de puertos, incluidos SSH, DNS personalizado y servicios internos si no agregas reglas.", Policy: "{\"grants\":[{\"src\":[\"autogroup:member\"],\"dst\":[\"autogroup:member\"],\"ip\":[\"80\",\"443\"]}]}", Source: "builtin"},
		{ID: "ssh-admin", Name: "SSH a servidores", Category: "Administración", Description: "Acceso limitado a tcp:22. Sustituye operador@ por un usuario real y asigna la etiqueta tag:servidor.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:servidor\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:servidor\"],\"ip\":[\"tcp:22\"]}]}", Source: "builtin"},
		{ID: "rdp-windows", Name: "Escritorio remoto Windows", Category: "Administración", Description: "Acceso limitado a tcp:3389, udp:3389. Sustituye operador@ por un usuario real y asigna la etiqueta tag:windows.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:windows\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:windows\"],\"ip\":[\"tcp:3389\",\"udp:3389\"]}]}", Source: "builtin"},
		{ID: "dns-interno", Name: "DNS interno TCP y UDP", Category: "Infraestructura", Description: "Acceso limitado a tcp:53, udp:53. Sustituye operador@ por un usuario real y asigna la etiqueta tag:dns.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:dns\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:dns\"],\"ip\":[\"tcp:53\",\"udp:53\"]}]}", Source: "builtin"},
		{ID: "gateway-https", Name: "Portal Gateway HTTPS", Category: "Infraestructura", Description: "Acceso limitado a tcp:443, tcp:8443. Sustituye operador@ por un usuario real y asigna la etiqueta tag:gateway.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:gateway\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:gateway\"],\"ip\":[\"tcp:443\",\"tcp:8443\"]}]}", Source: "builtin"},
		{ID: "nas-smb", Name: "Archivos NAS mediante SMB", Category: "Servicios", Description: "Acceso limitado a tcp:445. Sustituye operador@ por un usuario real y asigna la etiqueta tag:nas.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:nas\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:nas\"],\"ip\":[\"tcp:445\"]}]}", Source: "builtin"},
		{ID: "postgres", Name: "PostgreSQL de aplicaciones", Category: "Bases de datos", Description: "Acceso limitado a tcp:5432. Sustituye operador@ por un usuario real y asigna la etiqueta tag:database.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:database\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:database\"],\"ip\":[\"tcp:5432\"]}]}", Source: "builtin"},
		{ID: "mysql", Name: "MySQL / MariaDB", Category: "Bases de datos", Description: "Acceso limitado a tcp:3306. Sustituye operador@ por un usuario real y asigna la etiqueta tag:database.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:database\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:database\"],\"ip\":[\"tcp:3306\"]}]}", Source: "builtin"},
		{ID: "redis", Name: "Redis interno", Category: "Bases de datos", Description: "Acceso limitado a tcp:6379. Sustituye operador@ por un usuario real y asigna la etiqueta tag:database.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:database\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:database\"],\"ip\":[\"tcp:6379\"]}]}", Source: "builtin"},
		{ID: "monitorizacion", Name: "Prometheus y exporters", Category: "Infraestructura", Description: "Acceso limitado a tcp:9090, tcp:9100. Sustituye operador@ por un usuario real y asigna la etiqueta tag:monitoring.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:monitoring\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:monitoring\"],\"ip\":[\"tcp:9090\",\"tcp:9100\"]}]}", Source: "builtin"},
		{ID: "domotica", Name: "Home Assistant y MQTT TLS", Category: "Hogar", Description: "Acceso limitado a tcp:8123, tcp:8883. Sustituye operador@ por un usuario real y asigna la etiqueta tag:domotica.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:domotica\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:domotica\"],\"ip\":[\"tcp:8123\",\"tcp:8883\"]}]}", Source: "builtin"},
		{ID: "multimedia", Name: "Jellyfin / Plex", Category: "Hogar", Description: "Acceso limitado a tcp:8096, tcp:8920, tcp:32400. Sustituye operador@ por un usuario real y asigna la etiqueta tag:media.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:media\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:media\"],\"ip\":[\"tcp:8096\",\"tcp:8920\",\"tcp:32400\"]}]}", Source: "builtin"},
		{ID: "icmp-diagnostico", Name: "Diagnóstico ICMP", Category: "Diagnóstico", Description: "Acceso limitado a icmp:*. Sustituye operador@ por un usuario real y asigna la etiqueta tag:diagnostico.", Warning: "Borrador restrictivo: reemplaza identidades y revisa acceso administrativo antes de aplicar. Requiere soporte Grants en Headscale.", Policy: "{\"groups\":{\"group:operadores\":[\"operador@\"]},\"tagOwners\":{\"tag:diagnostico\":[\"group:operadores\"]},\"grants\":[{\"src\":[\"group:operadores\"],\"dst\":[\"tag:diagnostico\"],\"ip\":[\"icmp:*\"]}]}", Source: "builtin"},
	}, serviceBuiltins()...)
}
