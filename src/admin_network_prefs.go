package main

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"tailscale.com/ipn"
)

// GatewayPrefsChange solo modifica el nodo tsnet de este proceso, nunca nodos remotos.
func (a *adminController) GatewayPrefsChange(ctx context.Context, req admin.GatewayPrefsRequest) (any, error) {
	if !a.runtime.currentConfig().NetworkPanelEnabled {
		return nil, admin.ErrNetworkDisabled
	}
	server := a.networkServer()
	if server == nil {
		return nil, errors.New("Gateway tsnet desconectado")
	}
	client, err := server.LocalClient()
	if err != nil {
		return nil, err
	}
	masked := &ipn.MaskedPrefs{}
	changes := 0
	if req.AcceptRoutes != nil {
		masked.RouteAllSet = true
		masked.RouteAll = *req.AcceptRoutes
		changes++
	}
	if req.AcceptDNS != nil {
		masked.CorpDNSSet = true
		masked.CorpDNS = *req.AcceptDNS
		changes++
	}
	if req.ExitNodeAllowLANAccess != nil {
		masked.ExitNodeAllowLANAccessSet = true
		masked.ExitNodeAllowLANAccess = *req.ExitNodeAllowLANAccess
		changes++
	}
	if req.AdvertiseRoutes != nil {
		if len(*req.AdvertiseRoutes) > 32 {
			return nil, errors.New("demasiadas rutas")
		}
		routes := make([]netip.Prefix, 0, len(*req.AdvertiseRoutes))
		seen := map[string]bool{}
		for _, raw := range *req.AdvertiseRoutes {
			p, e := netip.ParsePrefix(raw)
			if e != nil || !p.IsValid() || p.Addr().IsMulticast() || p.Addr().IsLoopback() || p.Addr().IsUnspecified() && p.Bits() != 0 {
				return nil, errors.New("ruta CIDR inválida")
			}
			p = p.Masked()
			if seen[p.String()] {
				continue
			}
			seen[p.String()] = true
			routes = append(routes, p)
		}
		masked.AdvertiseRoutesSet = true
		masked.AdvertiseRoutes = routes
		changes++
	}
	if changes == 0 {
		return nil, errors.New("no se proporcionaron cambios")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := client.EditPrefs(ctx, masked)
	if err != nil {
		return nil, err
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-TSNET-PREFS", "ajustes actualizados: %d campos; anunciar rutas requiere aprobación en Headscale", changes)
	return map[string]any{"updated": true, "accept_routes": result.RouteAll, "accept_dns": result.CorpDNS, "exit_node_allow_lan_access": result.ExitNodeAllowLANAccess, "advertise_routes": result.AdvertiseRoutes}, nil
}
