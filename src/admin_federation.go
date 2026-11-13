package main

import (
	"context"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func federationHost(name, zone string) string {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	name = strings.TrimPrefix(name, "admin.")
	if !strings.Contains(name, ".") {
		name += "." + zone
	}
	label := strings.TrimSuffix(name, "."+zone)
	if label == name || label == "" || strings.Contains(label, ".") || normalizeDNSName(label) != label {
		return ""
	}
	return name
}
func (a *adminController) federationCandidates(ctx context.Context) ([]pki.Candidate, error) {
	cfg := a.runtime.currentConfig()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	byHost := map[string]*pki.Candidate{}
	add := func(name, origin string, addresses []string) {
		u, e := pki.PeerURL(origin)
		if e != nil {
			return
		}
		host := federationHost(u.Hostname(), cfg.BaseDomain)
		if host == "" || host == a.runtime.domain() {
			return
		}
		c := byHost[host]
		if c == nil {
			if len(byHost) >= 24 {
				return
			}
			c = &pki.Candidate{Name: name, Endpoint: "https://" + host, Addresses: []string{}, Targets: []pki.Target{}}
			byHost[host] = c
		}
		if u.Port() != "" {
			c.Endpoint = "https://" + net.JoinHostPort(host, u.Port())
		}
		c.Addresses = append(c.Addresses, addresses...)
	}
	if data, e := a.NetworkNodes(ctx); e == nil {
		if inv, ok := data.(network.Inventory); ok {
			for _, n := range inv.Nodes {
				name := n.GivenName
				if name == "" {
					name = n.Name
				}
				host := federationHost(name, cfg.BaseDomain)
				if host == "" {
					continue
				}
				ips := append([]string(nil), n.IPAddresses...)
				if n.Peer != nil {
					for _, ep := range append(n.Peer.Endpoints, n.Peer.CurrentEndpoint) {
						ip, _, e := net.SplitHostPort(ep)
						if e == nil {
							ips = append(ips, ip)
						}
					}
				}
				add(name, "https://"+host, ips)
			}
		}
	}
	if lan, e := pki.DiscoverLAN(ctx, cfg.DiscoveryPort); e == nil {
		for _, s := range lan {
			origin := s.Endpoint
			if u, e := pki.PeerURL(origin); e == nil && u.Port() == "" {
				u.Host = net.JoinHostPort(u.Hostname(), "443")
				origin = u.String()
			}
			add(s.Name, origin, []string{s.Address})
			if cfg.CAHTTPDownload && s.CAPort > 0 && !pki.IsTailnetIP(s.Address) {
				if u, e := pki.PeerURL(origin); e == nil {
					if c := byHost[federationHost(u.Hostname(), cfg.BaseDomain)]; c != nil {
						c.CATargets = append(c.CATargets, pki.Target{Origin: "http://" + net.JoinHostPort(u.Hostname(), strconv.Itoa(s.CAPort)), IP: s.Address, Network: "LAN HTTP"})
					}
				}
			}
		}
	}
	out := []pki.Candidate{}
	for _, c := range byHost {
		targets, e := pki.Plan(ctx, c.Endpoint, c.Addresses)
		if e != nil {
			c.Error = e.Error()
		} else {
			c.Targets = targets
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Endpoint < out[j].Endpoint })
	return out, nil
}
func (a *adminController) PKICandidates(ctx context.Context) (any, error) {
	candidates, err := a.federationCandidates(ctx)
	return map[string]any{"candidates": candidates, "lan": []any{}, "tailnet_candidates": []any{}, "note": "Tailscale primero (HTTPS 443); después HTTPS LAN en el puerto anunciado. HTTP LAN solo para CA si lo permites. Descargar no implica aprobar."}, err
}
func (a *adminController) PKIDownload(ctx context.Context, endpoint string) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	candidates, err := a.federationCandidates(ctx)
	if err != nil {
		return nil, err
	}
	store, err := pki.NewStore(a.runtime.currentConfig().DataDir)
	if err != nil {
		return nil, err
	}
	type result struct {
		Endpoint string      `json:"endpoint"`
		Anchor   *pki.Anchor `json:"anchor,omitempty"`
		Error    string      `json:"error,omitempty"`
	}
	requestedHost := ""
	if endpoint != "" {
		u, e := pki.PeerURL(endpoint)
		if e != nil {
			return nil, e
		}
		requestedHost = u.Hostname()
	}
	results := []result{}
	for _, c := range candidates {
		u, _ := pki.PeerURL(c.Endpoint)
		if endpoint == "" || u.Hostname() == requestedHost {
			results = append(results, result{Endpoint: c.Endpoint})
		}
	}
	if endpoint != "" && len(results) == 0 {
		return nil, errors.New("el candidato ya no está disponible; vuelve a descubrir gateways")
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			r := &results[i]
			for _, c := range candidates {
				if c.Endpoint != r.Endpoint {
					continue
				}
				if len(c.Targets) == 0 {
					r.Error = c.Error
					return
				}
				anchor, e := pki.DownloadCA(ctx, append(append([]pki.Target{}, c.Targets...), c.CATargets...), a.runtime.dialTailnet)
				if e != nil {
					r.Error = e.Error()
					return
				}
				anchor.Name = c.Name
				if strings.HasPrefix(anchor.Endpoint, "http://") {
					anchor.Endpoint = c.Endpoint
				}
				saved, e := store.ImportPending(anchor)
				if e != nil {
					r.Error = e.Error()
					return
				}
				r.Anchor = &saved
				return
			}
		}(i)
	}
	wg.Wait()
	return results, nil
}

// Prueba primero el DNS actual; descubre pistas LAN nuevas solo si hace falta.
// Una pista antigua sin verificar nunca permite omitir la validación del certificado.
func (a *adminController) peerTargets(ctx context.Context, origin string) ([]pki.Target, error) {
	u, err := pki.PeerURL(origin)
	if err != nil {
		return nil, err
	}
	host := strings.TrimPrefix(u.Hostname(), "admin.")
	u.Host = host
	if p, _ := url.Parse(origin); p.Port() != "" {
		u.Host = net.JoinHostPort(host, p.Port())
	}
	targets, initialErr := pki.Plan(ctx, u.String(), nil)
	candidates, _ := a.federationCandidates(ctx)
	for _, c := range candidates {
		v, _ := url.Parse(c.Endpoint)
		if v.Hostname() == host {
			if len(c.Targets) > 0 {
				return c.Targets, nil
			}
		}
	}
	return targets, initialErr
}
func (a *adminController) fetchPeerCatalog(ctx context.Context, origin, ca string) (portal.Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	var failures []error
	u, err := pki.PeerURL(origin)
	if err != nil {
		return portal.Catalog{}, err
	}
	host := strings.TrimPrefix(u.Hostname(), "admin.")
	port := u.Port()
	u.Host = host
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	initial, _ := pki.Plan(ctx, u.String(), nil)
	seen := map[string]bool{}
	try := func(targets []pki.Target) (portal.Catalog, bool) {
		for _, target := range targets {
			key := target.Origin + "@" + target.IP
			if seen[key] {
				continue
			}
			seen[key] = true
			c, e := portal.FetchTarget(ctx, target, ca, a.runtime.dialTailnet)
			if e == nil {
				return c, true
			}
			failures = append(failures, e)
		}
		return portal.Catalog{}, false
	}
	// Si DNS solo resuelve a LAN, consulta el inventario para priorizar la tailnet.
	if len(initial) > 0 && initial[0].Network == "Tailnet" {
		if c, ok := try(initial[:1]); ok {
			return c, nil
		}
	}
	targets, e := a.peerTargets(ctx, origin)
	if e != nil {
		failures = append(failures, e)
	}
	if c, ok := try(targets); ok {
		return c, nil
	}
	return portal.Catalog{}, errors.Join(failures...)
}
func (a *adminController) PKISync(ctx context.Context, peer pki.Anchor) (pki.Anchor, error) {
	if !peer.Approved {
		return peer, errors.New("la CA debe aprobarse antes de sincronizar")
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	targets, err := a.peerTargets(ctx, peer.Endpoint)
	if err != nil {
		return peer, err
	}
	var failures []error
	for _, target := range targets {
		for _, admin := range []bool{true, false} {
			t := target
			u, _ := pki.PeerURL(t.Origin)
			if admin {
				host := "admin." + u.Hostname()
				if u.Port() != "" {
					host = net.JoinHostPort(host, u.Port())
				}
				u.Host = host
				t.Origin = u.String()
			}
			raw, e := pki.GetPublic(ctx, t, "/pki/v1/public-ca", peer.PEM, false, a.runtime.dialTailnet, 64<<10)
			if e == nil {
				root, parseErr := pki.ParseRoot(raw)
				e = parseErr
				if e == nil && root.Fingerprint != peer.Fingerprint {
					e = errors.New("CA remota cambiada: nueva aprobación necesaria")
				}
				if e == nil {
					peer.LastSync = time.Now().UTC()
					return peer, nil
				}
			}
			failures = append(failures, e)
		}
	}
	return peer, errors.Join(failures...)
}
