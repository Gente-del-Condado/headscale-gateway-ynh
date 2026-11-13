package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"
)

// newDNSProvider crea la implementación DNS seleccionada mediante
// la configuración.
//
// La función devuelve la interfaz DNSProvider, por lo que el resto
// del Gateway no necesita conocer qué proveedor concreto se está
// utilizando.
//
// Cuando se añadan nuevos proveedores, únicamente será necesario
// añadir aquí la selección correspondiente y su implementación.
func newDNSProvider(cfg Config) (DNSProvider, error) {
	switch cfg.DNSProvider {
	case "none":
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-PROVIDER-FACTORY-001", "Proveedor DNS desactivado: no se sincronizarán registros DNS")
		return nil, nil

	case "cloudflare":
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-PROVIDER-FACTORY-002", "Proveedor DNS seleccionado: Cloudflare")
		return newCloudflareDNS(cfg.CloudflareDNSZoneID, cfg.CloudflareDNSToken, cfg.DNSZone), nil

	case "technitium":
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-PROVIDER-FACTORY-003", "Proveedor DNS seleccionado: Technitium")
		return newTechnitiumDNS(cfg), nil

	default:
		return nil, fmt.Errorf("proveedor DNS no soportado: %s", cfg.DNSProvider)
	}
}
