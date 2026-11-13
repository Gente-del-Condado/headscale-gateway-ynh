package main

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
)

func (a *adminController) HeadscaleGetPolicy(ctx context.Context) (network.PolicyDocument, error) {
	client, err := a.headscaleAdmin()
	if err != nil {
		return network.PolicyDocument{}, err
	}
	doc, err := client.GetPolicy(ctx)
	if err != nil {
		diagnostics.
			// No registra contenido de políticas ni claves API; solo el fallo saneado.
			Emit("admin", diagnostics.Warn, "HGW-HS-POLICY-READ", "lectura ACL fallida: %v", err)
	}
	return doc, err
}
func (a *adminController) HeadscaleSetPolicy(ctx context.Context, text, revision string) (network.PolicyDocument, error) {
	client, err := a.headscaleAdmin()
	if err != nil {
		return network.PolicyDocument{}, err
	}
	result, err := client.SetPolicy(ctx, text, revision)
	if err == nil {
		if revision == "missing" {
			diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-HS-POLICY-INIT-001", "política ACL inicializada explícitamente mediante la API")
		} else {
			diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-HS-POLICY-001", "política ACL actualizada mediante la API")
		}
	}
	return result, err
}

// Exporta alias DNS de servicios habilitados al formato extra_records_path.
// Este endpoint no modifica Headscale.
func (a *adminController) HeadscaleServiceDNS(ctx context.Context) ([]network.MagicDNSRecord, error) {
	cfg := a.runtime.currentConfig()
	snap := a.runtime.active.Load()
	if snap == nil {
		return nil, errors.New("el Gateway aún no tiene un nombre de nodo activo")
	}
	info, err := a.NetworkInfo(ctx)
	if err != nil {
		return nil, err
	}
	state, ok := info.(map[string]any)
	if !ok {
		return nil, errors.New("estado de red inválido")
	}
	var ipStrings []string
	switch ips := state["gateway_ips"].(type) {
	case []string:
		ipStrings = ips
	case []interface{}:
		for _, ip := range ips {
			if text, ok := ip.(string); ok {
				ipStrings = append(ipStrings, text)
			}
		}
	case []netip.Addr:
		for _, ip := range ips {
			ipStrings = append(ipStrings, ip.String())
		}
	}
	services, err := a.Services(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, service := range services {
		if service.Enabled == nil || *service.Enabled {
			names = append(names, strings.ToLower(service.Name))
		}
	}
	adminName := ""
	if cfg.AdminEnabled && cfg.AdminDomainMode != "off" {
		adminName = cfg.AdminSubdomain
	}
	return network.BuildMagicDNSRecords(snap.domain, ipStrings, names, adminName)
}
