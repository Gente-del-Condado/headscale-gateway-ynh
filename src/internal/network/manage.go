package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// HeadscaleAdmin es un cliente limitado de la API administrativa v1 de Headscale.
// Los endpoints y contenidos permitidos están predefinidos; el navegador no
// puede elegir métodos, servidores ni rutas remotas arbitrarias.
type HeadscaleAdmin struct{ BaseURL, APIKey string }

// HeadscaleHTTPError conserva el estado HTTP remoto sin exponer el cuerpo,
// porque los diagnósticos de Headscale pueden contener información sensible.
type HeadscaleHTTPError struct{ StatusCode int }

// ErrPolicyNotFound solo se devuelve ante el diagnóstico explícito de política
// ausente de Headscale. Otros errores HTTP 500 no se interpretan como una
// base de datos ACL vacía.
var ErrPolicyNotFound = errors.New("acl policy not found")

func (e *HeadscaleHTTPError) Error() string {
	return fmt.Sprintf("Headscale devuelve HTTP %d; verifica la versión, el permiso y los datos", e.StatusCode)
}

var nodeIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)
var userNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@-]{0,62}$`)
var tagPattern = regexp.MustCompile(`^tag:[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

func (a HeadscaleAdmin) call(ctx context.Context, method, endpoint string, body any) (json.RawMessage, error) {
	if err := ValidateURL(a.BaseURL); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, errors.New("la API key de Headscale no está configurada")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.BaseURL, "/")+"/api/v1/"+endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("error al consultar Headscale: %w", err)
	}
	defer resp.Body.Close()
	// No propaga cuerpos de error remotos: las trazas del proxy pueden incluir tokens.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if endpoint == "policy" && method == http.MethodGet && (resp.StatusCode == 404 || resp.StatusCode == 500) {
			// Inspect an explicitly bounded diagnostic, never pass its contents
			// into logs, returned errors or an HTTP response.
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			if utf8.Valid(snippet) && strings.Contains(strings.ToLower(string(snippet)), "acl policy not found") {
				return nil, fmt.Errorf("%w: %w", ErrPolicyNotFound, &HeadscaleHTTPError{StatusCode: resp.StatusCode})
			}
		}
		return nil, &HeadscaleHTTPError{StatusCode: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("respuesta de Headscale demasiado grande")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("Headscale devolvió una respuesta JSON inválida")
	}
	return raw, nil
}

func checkNodeID(id string) error {
	if !nodeIDPattern.MatchString(id) {
		return errors.New("identificador de nodo inválido")
	}
	return nil
}
func checkName(name string) error {
	if !userNamePattern.MatchString(name) {
		return errors.New("nombre inválido (letras, números, guiones y puntos)")
	}
	return nil
}
func checkTags(tags []string) error {
	if len(tags) > 32 {
		return errors.New("demasiadas etiquetas")
	}
	for _, tag := range tags {
		if !tagPattern.MatchString(tag) {
			return fmt.Errorf("etiqueta inválida: %q", tag)
		}
	}
	return nil
}
func checkRoutes(routes []string) error {
	if len(routes) > 64 {
		return errors.New("demasiadas rutas")
	}
	for _, route := range routes {
		if _, err := netip.ParsePrefix(route); err != nil {
			return fmt.Errorf("ruta CIDR inválida: %q", route)
		}
	}
	return nil
}
func (a HeadscaleAdmin) Users(ctx context.Context) (json.RawMessage, error) {
	return a.call(ctx, "GET", "user", nil)
}
func (a HeadscaleAdmin) CreateUser(ctx context.Context, name string) (json.RawMessage, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "user", map[string]string{"name": name})
}
func (a HeadscaleAdmin) RenameUser(ctx context.Context, id, name string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	if err := checkName(name); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "user/"+id+"/rename/"+url.PathEscape(name), nil)
}
func (a HeadscaleAdmin) DeleteUser(ctx context.Context, id string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	return a.call(ctx, "DELETE", "user/"+id, nil)
}
func (a HeadscaleAdmin) PreAuthKeys(ctx context.Context) (json.RawMessage, error) {
	return a.call(ctx, "GET", "preauthkey", nil)
}
func (a HeadscaleAdmin) CreatePreAuthKey(ctx context.Context, user string, reusable, ephemeral bool, hours int, tags []string) (json.RawMessage, error) {
	if user != "" {
		if err := checkNodeID(user); err != nil {
			return nil, err
		}
	}
	if err := checkTags(tags); err != nil {
		return nil, err
	}
	if hours < 1 || hours > 24*90 {
		return nil, errors.New("duración entre 1 hora y 90 días")
	}
	req := map[string]any{"reusable": reusable, "ephemeral": ephemeral, "expiration": time.Now().UTC().Add(time.Duration(hours) * time.Hour).Format(time.RFC3339), "aclTags": tags}
	if user != "" {
		req["user"] = user
	}
	return a.call(ctx, "POST", "preauthkey", req)
}
func (a HeadscaleAdmin) ExpirePreAuthKey(ctx context.Context, id string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "preauthkey/expire", map[string]string{"id": id})
}
func (a HeadscaleAdmin) DeletePreAuthKey(ctx context.Context, id string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	return a.call(ctx, "DELETE", "preauthkey?id="+url.QueryEscape(id), nil)
}
func (a HeadscaleAdmin) RenameNode(ctx context.Context, id, name string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	if err := checkName(name); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "node/"+id+"/rename/"+url.PathEscape(name), nil)
}
func (a HeadscaleAdmin) ExpireNode(ctx context.Context, id string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "node/"+id+"/expire", map[string]any{})
}
func (a HeadscaleAdmin) DeleteNode(ctx context.Context, id string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	return a.call(ctx, "DELETE", "node/"+id, nil)
}
func (a HeadscaleAdmin) SetTags(ctx context.Context, id string, tags []string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	if err := checkTags(tags); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "node/"+id+"/tags", map[string]any{"tags": tags})
}
func (a HeadscaleAdmin) SetApprovedRoutes(ctx context.Context, id string, routes []string) (json.RawMessage, error) {
	if err := checkNodeID(id); err != nil {
		return nil, err
	}
	if err := checkRoutes(routes); err != nil {
		return nil, err
	}
	return a.call(ctx, "POST", "node/"+id+"/approve_routes", map[string]any{"routes": routes})
}
