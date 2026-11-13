package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
)

func (a *adminController) headscaleAdmin() (network.HeadscaleAdmin, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.NetworkPanelEnabled {
		return network.HeadscaleAdmin{}, admin.ErrNetworkDisabled
	}
	if err := network.ValidateURL(cfg.HeadscaleURL); err != nil {
		return network.HeadscaleAdmin{}, err
	}
	if cfg.HeadscaleAPIKey == "" {
		return network.HeadscaleAdmin{}, errors.New("configura una API key de Headscale y reinicia el Gateway")
	}
	return network.HeadscaleAdmin{BaseURL: cfg.HeadscaleURL, APIKey: cfg.HeadscaleAPIKey}, nil
}
func (a *adminController) HeadscaleUsers(ctx context.Context) (any, error) {
	h, err := a.headscaleAdmin()
	if err != nil {
		return nil, err
	}
	return h.Users(ctx)
}
func (a *adminController) HeadscalePreAuthKeys(ctx context.Context) (any, error) {
	h, err := a.headscaleAdmin()
	if err != nil {
		return nil, err
	}
	raw, err := h.PreAuthKeys(ctx)
	if err != nil {
		return nil, err
	}
	// Nunca devuelve secretos de registro en un endpoint de listado.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.New("lista de claves inválida")
	}
	var keys []map[string]any
	if err := json.Unmarshal(obj["preAuthKeys"], &keys); err != nil {
		if err := json.Unmarshal(obj["preauthkeys"], &keys); err != nil {
			return nil, errors.New("lista de claves no reconocida en esta versión de Headscale")
		}
	}
	for _, key := range keys {
		delete(key, "key")
	}
	return map[string]any{"preAuthKeys": keys}, nil
}
func (a *adminController) HeadscaleManage(ctx context.Context, cmd admin.HeadscaleAction) (any, error) {
	h, err := a.headscaleAdmin()
	if err != nil {
		return nil, err
	}
	diagnostics.
		// Do not log names, API keys, response bodies or generated preauth keys.
		Emit("admin", diagnostics.InfoLevel, "HGW-HS-ADMIN-001", "operación administrativa Headscale: %s", cmd.Action)
	switch cmd.Action {
	case "node.rename":
		return h.RenameNode(ctx, cmd.ID, cmd.Name)
	case "node.expire":
		return h.ExpireNode(ctx, cmd.ID)
	case "node.delete":
		return h.DeleteNode(ctx, cmd.ID)
	case "node.tags":
		return h.SetTags(ctx, cmd.ID, cmd.Tags)
	case "node.routes":
		return h.SetApprovedRoutes(ctx, cmd.ID, cmd.Routes)
	case "user.create":
		return h.CreateUser(ctx, cmd.Name)
	case "user.rename":
		return h.RenameUser(ctx, cmd.ID, cmd.Name)
	case "user.delete":
		return h.DeleteUser(ctx, cmd.ID)
	case "key.create":
		return h.CreatePreAuthKey(ctx, cmd.User, cmd.Reusable, cmd.Ephemeral, cmd.Hours, cmd.Tags)
	case "key.expire":
		return h.ExpirePreAuthKey(ctx, cmd.ID)
	case "key.delete":
		return h.DeletePreAuthKey(ctx, cmd.ID)
	default:
		return nil, errors.New("operación Headscale no permitida")
	}
}
