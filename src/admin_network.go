package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

// Esta pestaña distingue control-plane (Headscale) y data-plane (tsnet).
// Headscale permite gestionar identidad y rutas anunciadas, pero no las preferencias locales del cliente Tailscale.
func (a *adminController) networkServer() *tsnet.Server {
	a.vpn.mu.Lock()
	defer a.vpn.mu.Unlock()
	if a.vpn.server != nil {
		return a.vpn.server
	}
	return a.vpn.connecting
}
func (a *adminController) NetworkInfo(ctx context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	info := map[string]any{"enabled": cfg.NetworkPanelEnabled, "headscale_url": cfg.HeadscaleURL, "api_key_configured": cfg.HeadscaleAPIKey != "", "gateway_vpn_state": "offline", "capabilities": map[string]bool{"inventory": true, "ping": true, "remote_client_preferences": false, "headscale_route_approval": true}}
	info["pause_allowed"] = ctx.Value(ingressContextKey{}) != true
	if ctx.Value(ingressContextKey{}) == true {
		info["pause_block_reason"] = pauseFromVPNMessage
	}
	if !cfg.NetworkPanelEnabled {
		return info, nil
	}
	a.vpn.mu.Lock()
	cachedURL := a.vpn.loginURL
	a.vpn.mu.Unlock()
	if validGatewayLoginURL(cfg.HeadscaleURL, cachedURL) {
		info["login_url"] = cachedURL
	}
	if s := a.networkServer(); s != nil {
		c, e := s.LocalClient()
		if e == nil {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			// Estas preferencias pertenecen EXCLUSIVAMENTE al tsnet integrado.
			if prefs, prefErr := c.GetPrefs(ctx); prefErr == nil && prefs != nil {
				routes := make([]string, 0, len(prefs.AdvertiseRoutes))
				for _, prefix := range prefs.AdvertiseRoutes {
					routes = append(routes, prefix.String())
				}
				info["gateway_preferences"] = map[string]any{
					"accept_routes": prefs.RouteAll, "accept_dns": prefs.CorpDNS,
					"advertise_routes": routes, "exit_node_allow_lan_access": prefs.ExitNodeAllowLANAccess,
				}
			}
			if st, e := c.Status(ctx); e == nil && st != nil {
				info["gateway_vpn_state"] = st.BackendState
				if st.BackendState == "Running" {
					delete(info, "login_url")
				}
				if st.BackendState != "Running" && st.AuthURL != "" && validGatewayLoginURL(cfg.HeadscaleURL, st.AuthURL) {
					info["login_url"] = st.AuthURL
				}
				info["gateway_ips"] = st.TailscaleIPs
				info["gateway_health"] = st.Health
				if st.Self != nil {
					info["gateway_dns_name"] = st.Self.DNSName
					info["gateway_derp"] = st.Self.Relay
				}
			}
		}
	}
	if a.vpn.isPaused() {
		info["gateway_vpn_state"] = "Stopped"
		info["manually_paused"] = true
		delete(info, "login_url")
	}
	return info, nil
}
func (a *adminController) NetworkNodes(ctx context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.NetworkPanelEnabled {
		return nil, admin.ErrNetworkDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	inventory, err := network.List(ctx, cfg.HeadscaleURL, cfg.HeadscaleAPIKey)
	if err != nil {
		return nil, err
	}
	if s := a.networkServer(); s != nil {
		if c, e := s.LocalClient(); e == nil {
			if st, e := c.Status(ctx); e == nil && st != nil {
				peers := map[string]*network.Peer{}
				for _, p := range st.Peer {
					if p == nil {
						continue
					}
					np := &network.Peer{Hostname: p.HostName, OS: p.OS, DNSName: strings.TrimSuffix(p.DNSName, "."), Online: p.Online, Active: p.Active, Relay: p.Relay, PeerRelay: p.PeerRelay, CurrentEndpoint: p.CurAddr, Endpoints: p.Addrs, RxBytes: p.RxBytes, TxBytes: p.TxBytes, LastHandshake: when(p.LastHandshake), LastWrite: when(p.LastWrite), LastSeen: when(p.LastSeen), ExitNodeOption: p.ExitNodeOption}
					for _, ip := range p.TailscaleIPs {
						peers[ip.String()] = np
					}
				}
				if st.Self != nil {
					self := st.Self
					np := &network.Peer{Hostname: self.HostName, OS: self.OS, DNSName: strings.TrimSuffix(self.DNSName, "."), Online: self.Online, Active: self.Active, Relay: self.Relay, CurrentEndpoint: self.CurAddr, Endpoints: self.Addrs, RxBytes: self.RxBytes, TxBytes: self.TxBytes, LastHandshake: when(self.LastHandshake), LastWrite: when(self.LastWrite), LastSeen: when(self.LastSeen), ExitNodeOption: self.ExitNodeOption}
					for _, ip := range self.TailscaleIPs {
						peers[ip.String()] = np
					}
				}
				for i := range inventory.Nodes {
					for _, ip := range inventory.Nodes[i].IPAddresses {
						if peer := peers[ip]; peer != nil {
							inventory.Nodes[i].Peer = peer
							break
						}
					}
				}
			}
		}
	}
	return inventory, nil
}
func when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
func (a *adminController) NetworkPing(ctx context.Context, nodeID, mode string) (any, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.NetworkPanelEnabled {
		return nil, admin.ErrNetworkDisabled
	}
	if mode != "disco" && mode != "tsmp" && mode != "icmp" && diagnosticPort(mode) == "" && mode != "dns-reverse" {
		return nil, errors.New("tipo de prueba no admitido")
	}
	s := a.networkServer()
	if s == nil {
		return nil, errors.New("instancia VPN del Gateway desconectada")
	}
	if nodeID == "" {
		return nil, errors.New("node_id obligatorio")
	}
	ctx, cancel := context.WithTimeout(ctx, 14*time.Second)
	defer cancel()
	inv, err := network.List(ctx, cfg.HeadscaleURL, cfg.HeadscaleAPIKey)
	if err != nil {
		return nil, err
	}
	var target netip.Addr
	for _, n := range inv.Nodes {
		if n.ID != nodeID {
			continue
		}
		for _, ip := range n.IPAddresses {
			if parsed, e := netip.ParseAddr(ip); e == nil && parsed.IsValid() && parsed.Is6() == false {
				target = parsed
				break
			}
		}
		if !target.IsValid() {
			for _, ip := range n.IPAddresses {
				if parsed, e := netip.ParseAddr(ip); e == nil && parsed.IsValid() {
					target = parsed
					break
				}
			}
		}
		break
	}
	if !target.IsValid() {
		return nil, errors.New("nodo no encontrado o sin IP VPN válida")
	}
	if mode == "dns-reverse" {
		started := time.Now()
		names, err := net.DefaultResolver.LookupAddr(ctx, target.String())
		result := map[string]any{"node_id": nodeID, "ip": target.String(), "mode": mode, "names": names, "latency_ms": time.Since(started).Milliseconds(), "note": "Consulta PTR desde el resolver del SO del Gateway; la ausencia de PTR no implica que el nodo esté caído."}
		if err != nil {
			result["error"] = err.Error()
		}
		return result, nil
	}
	if port := diagnosticPort(mode); port != "" {
		started := time.Now()
		result := map[string]any{"node_id": nodeID, "ip": target.String(), "port": port, "mode": mode, "egress": "tailnet"}
		conn, err := s.Dial(ctx, "tcp", net.JoinHostPort(target.String(), port))
		result["latency_ms"] = time.Since(started).Milliseconds()
		if err != nil {
			result["error"] = err.Error()
			return result, nil
		}
		defer conn.Close()
		result["connected"] = true
		if strings.HasPrefix(mode, "tls-") {
			secured := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.String()})
			if err = secured.HandshakeContext(ctx); err != nil {
				result["error"] = err.Error()
				result["note"] = "TCP disponible, pero TLS no se valida por IP con el almacén del SO. Comprueba SAN, confianza y si el servicio exige un nombre DNS."
				return result, nil
			}
			state := secured.ConnectionState()
			result["tls_version"] = tls.VersionName(state.Version)
			if len(state.PeerCertificates) > 0 {
				cert := state.PeerCertificates[0]
				result["expires"] = cert.NotAfter
				result["dns_names"] = cert.DNSNames
				result["issuer"] = cert.Issuer.String()
			}
		}
		return result, nil
	}
	client, err := s.LocalClient()
	if err != nil {
		return nil, err
	}
	typ := tailcfg.PingDisco
	if mode == "tsmp" {
		typ = tailcfg.PingTSMP
	}
	if mode == "icmp" {
		typ = tailcfg.PingICMP
	}
	res, err := client.Ping(ctx, target, typ)
	if err != nil {
		return nil, fmt.Errorf("ping Tailscale: %w", err)
	}
	if res == nil {
		return nil, errors.New("el nodo no ha respondido")
	}
	return map[string]any{"node_id": nodeID, "ip": target.String(), "mode": mode, "latency_ms": res.LatencySeconds * 1000, "endpoint": res.Endpoint, "derp_region": res.DERPRegionCode, "error": res.Err}, nil
}

// NetworkLogin devuelve el enlace visible para el navegador aunque el login
// esté todavía en marcha y el backend aún no haya actualizado AuthURL.
func (a *adminController) NetworkLogin(ctx context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.NetworkPanelEnabled {
		return nil, admin.ErrNetworkDisabled
	}
	if cfg.HeadscaleURL == "" {
		return nil, errors.New("configura la URL de Headscale y reinicia el Gateway")
	}
	a.vpn.actionMu.Lock()
	defer a.vpn.actionMu.Unlock()
	if a.vpn.isPaused() {
		if s := a.networkServer(); s != nil {
			c, e := s.LocalClient()
			if e != nil {
				return nil, e
			}
			call, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			if _, e = c.EditPrefs(call, &ipn.MaskedPrefs{WantRunningSet: true, Prefs: ipn.Prefs{WantRunning: true}}); e != nil {
				return nil, e
			}
		}
		if e := a.saveVPNPause(false); e != nil {
			return nil, e
		}
		return map[string]any{"state": "Starting", "message": "Reconectando con la identidad guardada"}, nil
	}

	s := a.networkServer()
	if s == nil {
		return nil, errors.New("tsnet aún no está disponible; comprueba que el Gateway esté iniciado")
	}
	c, err := s.LocalClient()
	if err != nil {
		return nil, fmt.Errorf("cliente tsnet todavía inicializándose: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	st, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}
	if st != nil && st.BackendState == "Running" {
		return map[string]any{"state": "Running", "message": "Gateway conectado a Headscale"}, nil
	}
	// Cuando tsnet.Up() ya ha iniciado un login interactivo, no forzar
	// un segundo intento: reutilizar AuthURL y el enlace capturado.
	result := map[string]any{"state": "NeedsLogin", "message": "Autorización pendiente: abre el enlace mostrado. Si aún no está disponible, actualiza el estado."}
	if st != nil {
		result["state"] = st.BackendState
	}
	if st != nil && validGatewayLoginURL(cfg.HeadscaleURL, st.AuthURL) {
		result["login_url"] = st.AuthURL
		return result, nil
	}
	a.vpn.mu.Lock()
	cached := a.vpn.loginURL
	a.vpn.mu.Unlock()
	if validGatewayLoginURL(cfg.HeadscaleURL, cached) {
		result["login_url"] = cached
		return result, nil
	}
	if err := c.StartLoginInteractive(ctx); err != nil {
		return nil, fmt.Errorf("solicitando autorización: %w", err)
	}
	// Puede publicarse unos instantes después del POST.
	result["message"] = "Inicio de sesión solicitado. El enlace aparecerá en el panel automáticamente."
	return result, nil
}

// Solo servicios predefinidos: el navegador no puede elegir destinos ni URL arbitrarios.
func diagnosticPort(mode string) string {
	switch mode {
	case "tcp-22":
		return "22"
	case "tcp-53":
		return "53"
	case "tcp-80":
		return "80"
	case "tcp-443", "tls-443":
		return "443"
	case "tcp-445":
		return "445"
	case "tcp-3389":
		return "3389"
	case "tcp-5432":
		return "5432"
	case "tcp-8123":
		return "8123"
	case "tcp-8443", "tls-8443":
		return "8443"
	}
	return ""
}
