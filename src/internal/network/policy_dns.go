package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tailscale/hujson"
	"strings"
	"unicode/utf8"
)

// PolicyDocument contiene la política huJSON actual y un token de concurrencia.
// El Gateway no interpreta la política; Headscale es la autoridad que la valida.
type PolicyDocument struct {
	Policy    string `json:"policy"`
	Revision  string `json:"revision"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Missing   bool   `json:"missing,omitempty"`
}

func policyRevision(policy string) string {
	sum := sha256.Sum256([]byte(policy))
	return hex.EncodeToString(sum[:])
}

func validPolicy(policy string) error {
	if strings.TrimSpace(policy) == "" || len(policy) > 100*1024 || !utf8.ValidString(policy) {
		return errors.New("la política está vacía, es demasiado grande o no es UTF-8")
	}
	normalized, err := hujson.Standardize([]byte(policy))
	if err != nil {
		return fmt.Errorf("formato huJSON inválido: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &object); err != nil || object == nil {
		return errors.New("la política ACL debe ser un objeto huJSON")
	}
	// {} permite todo en Headscale; solo se admite con aviso explícito en la interfaz.
	return nil
}

func (a HeadscaleAdmin) GetPolicy(ctx context.Context) (PolicyDocument, error) {
	raw, err := a.call(ctx, "GET", "policy", nil)
	if err != nil {
		if errors.Is(err, ErrPolicyNotFound) {
			return PolicyDocument{Missing: true, Revision: "missing"}, nil
		}
		var upstream *HeadscaleHTTPError
		if errors.As(err, &upstream) {
			switch upstream.StatusCode {
			case 401, 403:
				return PolicyDocument{}, fmt.Errorf("Headscale no autoriza consultar ACL (HTTP %d); comprueba la clave API: %w", upstream.StatusCode, err)
			case 404:
				return PolicyDocument{}, fmt.Errorf("Headscale responde 404 al consultar ACL: comprueba que esta versión exponga /api/v1/policy y que exista una política; revisa policy.mode y policy.path: %w", err)
			case 500, 503:
				return PolicyDocument{}, fmt.Errorf("Headscale no pudo recuperar la política ACL (HTTP %d): consulta los logs del servidor y verifica policy.mode, policy.path y si existe una política inicializada: %w", upstream.StatusCode, err)
			}
		}
		return PolicyDocument{}, fmt.Errorf("error consultando política ACL en Headscale: %w", err)
	}
	var res struct {
		Policy    string `json:"policy"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return PolicyDocument{}, errors.New("formato de política no reconocido")
	}
	if strings.TrimSpace(res.Policy) == "" {
		return PolicyDocument{Missing: true, Revision: "missing"}, nil
	}
	return PolicyDocument{Policy: res.Policy, Revision: policyRevision(res.Policy), UpdatedAt: res.UpdatedAt}, nil
}

// SetPolicy exige el hash de la política leída para evitar sobrescribir cambios
// de otras consolas. No es una comparación y sustitución atómica en Headscale:
// las escrituras externas concurrentes aún pueden competir.
func (a HeadscaleAdmin) SetPolicy(ctx context.Context, policy, expected string) (PolicyDocument, error) {
	if err := validPolicy(policy); err != nil {
		return PolicyDocument{}, err
	}
	if len(expected) != 64 && expected != "missing" {
		return PolicyDocument{}, errors.New("primero carga la política actual")
	}
	current, err := a.GetPolicy(ctx)
	if err != nil {
		return PolicyDocument{}, err
	}
	if expected != current.Revision {
		return PolicyDocument{}, errors.New("la política cambió en Headscale; vuelve a cargarla antes de guardar")
	}
	if current.Missing {
		normalized, err := hujson.Standardize([]byte(policy))
		if err != nil {
			return PolicyDocument{}, err
		}
		var basic map[string]json.RawMessage
		if err = json.Unmarshal(normalized, &basic); err != nil {
			return PolicyDocument{}, err
		}
		// La primera inicialización exige una política explícitamente abierta o vacía
		// que deniegue todo. Los demás cambios necesitan una revisión legible.
		if len(basic) > 0 {
			grants, ok := basic["grants"]
			if len(basic) != 1 || !ok || string(grants) != "[]" {
				return PolicyDocument{}, errors.New("inicializa primero con {} o con grants: []; luego edita la política")
			}
		}
	}
	if _, err = a.call(ctx, "PUT", "policy", map[string]string{"policy": policy}); err != nil {
		return PolicyDocument{}, err
	}
	return a.GetPolicy(ctx)
}
