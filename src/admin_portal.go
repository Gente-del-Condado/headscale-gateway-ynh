package main

import (
	"context"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"sort"
	"strings"
	"sync"
	"time"
)

func (a *adminController) PortalGet(ctx context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	peers, err := store.List()
	if err != nil {
		return nil, err
	}
	statuses := []portalPeerStatus{}
	a.runtime.portalMu.Lock()
	defer a.runtime.portalMu.Unlock()
	for _, peer := range peers {
		if !peer.Approved || peer.Endpoint == "" {
			continue
		}
		s := a.runtime.portalCache[peer.Fingerprint]
		s.Peer = peer.Fingerprint
		s.Name = peer.Name
		s.Endpoint = peer.Endpoint
		statuses = append(statuses, s)
	}
	local := localPortalEntries(cfg, a.runtime.domain(), "", false)
	// Incluye los servicios ocultos en el inventario administrativo.
	names := []string{}
	for name := range cfg.Services {
		names = append(names, normalizeServiceName(name))
	}
	sort.Strings(names)
	return map[string]any{"config": cfg.Portal, "local": local, "local_names": names, "peers": statuses, "url": "https://" + a.runtime.domain() + "/"}, nil
}
func (a *adminController) PortalSave(ctx context.Context, c portal.Config) (any, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	cfg := a.runtime.currentConfig()
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	for _, selected := range c.Selected {
		peer, err := store.Get(selected.Peer)
		if err != nil || !peer.Approved {
			return nil, errors.New("el Gateway seleccionado ya no está aprobado")
		}
		// Retain existing choices while offline, but new choices require a verified catalog.
		existing := false
		for _, old := range cfg.Portal.Selected {
			if old == selected {
				existing = true
			}
		}
		if existing {
			continue
		}
		a.runtime.portalMu.Lock()
		cache := a.runtime.portalCache[selected.Peer]
		a.runtime.portalMu.Unlock()
		found := false
		if cache.Error == "" && cache.Endpoint == peer.Endpoint && time.Since(cache.Checked) < 3*time.Minute {
			for _, entry := range cache.Catalog.Services {
				if entry.ID == selected.ID {
					found = true
				}
			}
		}
		if !found {
			return nil, errors.New("actualiza el catálogo antes de añadir el servicio remoto")
		}
	}
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err = db.PutJSON(portal.SettingsKey, c); err != nil {
		return nil, err
	}
	cfg.Portal = c
	a.runtime.cfg.Store(&cfg)
	return map[string]bool{"saved": true}, nil
}
func (a *adminController) PortalSync(ctx context.Context) (any, error) {
	return a.syncPortal(ctx, false)
}
func (a *adminController) syncPortal(ctx context.Context, selectedOnly bool) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cfg := a.runtime.currentConfig()
	if !cfg.Portal.Federation {
		return nil, errors.New("activa el módulo de federación del portal")
	}
	if !a.runtime.portalSyncMu.TryLock() {
		return nil, errors.New("ya hay una actualización de catálogos en curso")
	}
	defer a.runtime.portalSyncMu.Unlock()
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	peers, err := store.List()
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, s := range cfg.Portal.Selected {
		wanted[s.Peer] = true
	}
	count := 0
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, peer := range peers {
		if !peer.Approved || peer.Endpoint == "" || selectedOnly && !wanted[peer.Fingerprint] {
			continue
		}
		if count >= 32 || ctx.Err() != nil {
			break
		}
		count++
		slots <- struct{}{}
		workers.Add(1)
		go func(peer pki.Anchor) {
			defer workers.Done()
			defer func() { <-slots }()
			catalog, e := a.fetchPeerCatalog(ctx, peer.Endpoint, peer.PEM)
			next := portalPeerStatus{Peer: peer.Fingerprint, Name: peer.Name, Endpoint: peer.Endpoint, Checked: time.Now().UTC(), Catalog: catalog}
			if e != nil {
				next.Error = e.Error()
			}
			// Comprueba otra vez la aprobación tras la E/S: una revocación concurrente prevalece.
			current, e := store.Get(peer.Fingerprint)
			if e != nil || !current.Approved || current.Endpoint != peer.Endpoint {
				return
			}
			a.runtime.portalMu.Lock()
			if a.runtime.portalCache == nil {
				a.runtime.portalCache = map[string]portalPeerStatus{}
			}
			a.runtime.portalCache[peer.Fingerprint] = next
			a.runtime.portalMu.Unlock()
		}(peer)
	}
	workers.Wait()
	return a.PortalGet(ctx)
}
func (a *adminController) portalRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cfg := a.runtime.currentConfig()
		if !cfg.Portal.Federation || len(cfg.Portal.Selected) == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, _ = a.syncPortal(ctx, true)
		cancel()
	}
}

type portalCandidate struct {
	Name      string         `json:"name"`
	Endpoint  string         `json:"endpoint"`
	Source    string         `json:"source"`
	Reachable bool           `json:"reachable"`
	Error     string         `json:"error,omitempty"`
	Catalog   portal.Catalog `json:"catalog"`
}

func (a *adminController) PortalDiscover(ctx context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.Portal.Discovery {
		return nil, errors.New("activa el módulo de descubrimiento del portal")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	candidates, err := a.federationCandidates(ctx)
	if err != nil {
		return nil, err
	}
	approved := map[string]string{}
	if store, e := pki.NewStore(cfg.DataDir); e == nil {
		if peers, e := store.List(); e == nil {
			for _, p := range peers {
				if p.Approved {
					u, _ := pki.PeerURL(p.Endpoint)
					if u != nil {
						approved[strings.TrimPrefix(u.Hostname(), "admin.")] = p.PEM
					}
				}
			}
		}
	}
	out := make([]portalCandidate, len(candidates))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, c := range candidates {
		out[i] = portalCandidate{Name: c.Name, Endpoint: c.Endpoint, Source: "Tailnet → LAN"}
		wg.Add(1)
		go func(i int, c pki.Candidate) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			u, _ := pki.PeerURL(c.Endpoint)
			var failures []error
			for _, target := range c.Targets {
				catalog, e := portal.FetchTarget(ctx, target, approved[u.Hostname()], a.runtime.dialTailnet)
				if e == nil {
					out[i].Reachable = true
					out[i].Catalog = catalog
					out[i].Endpoint = target.Origin
					return
				}
				failures = append(failures, e)
			}
			if e := errors.Join(failures...); e != nil {
				out[i].Error = e.Error()
			} else {
				out[i].Error = c.Error
			}
		}(i, c)
	}
	wg.Wait()
	return out, nil
}

// PortalProbe lee un nodo aprobado sin cambiar selecciones ni reenviar credenciales
// de este Gateway. Fetch conserva las comprobaciones TLS, de destino y de tsnet.
func (a *adminController) PortalProbe(ctx context.Context, fingerprint string) (any, error) {
	cfg := a.runtime.currentConfig()
	if !cfg.Portal.Federation {
		return nil, errors.New("activa la federación del portal antes de consultar un peer")
	}
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	peer, err := store.Get(fingerprint)
	if err != nil || !peer.Approved {
		return nil, errors.New("el Gateway seleccionado ya no está aprobado")
	}
	catalog, err := a.fetchPeerCatalog(ctx, peer.Endpoint, peer.PEM)
	if err != nil {
		return nil, err
	}
	current, err := store.Get(fingerprint)
	if err != nil || !current.Approved || current.Endpoint != peer.Endpoint {
		return nil, errors.New("el Gateway seleccionado ya no está aprobado")
	}
	return catalog, nil
}
