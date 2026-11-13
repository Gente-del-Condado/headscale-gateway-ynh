package main

import (
	"encoding/json"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type portalPeerStatus struct {
	Peer     string         `json:"peer"`
	Name     string         `json:"name"`
	Endpoint string         `json:"endpoint"`
	Checked  time.Time      `json:"checked"`
	Error    string         `json:"error,omitempty"`
	Catalog  portal.Catalog `json:"catalog"`
}

func localPortalEntries(cfg Config, domain, port string, shared bool) []portal.Entry {
	out := []portal.Entry{}
	for name := range cfg.Services {
		id := normalizeServiceName(name)
		item := cfg.Portal.Local[id]
		if item.Hidden || shared && !item.Share {
			continue
		}
		label := item.Label
		if label == "" {
			label = strings.ReplaceAll(id, "-", " ")
		}
		host := id + "." + domain
		if port != "" && port != "443" {
			host = net.JoinHostPort(host, port)
		}
		out = append(out, portal.Entry{ID: id, Name: label, Description: item.Description, Group: item.Group, URL: "https://" + host, Gateway: domain})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (r *gatewayRuntime) remotePortalEntries(cfg Config) []portal.Entry {
	out := []portal.Entry{}
	if !cfg.Portal.Federation {
		return out
	}
	store, err := pki.NewStore(cfg.DataDir)
	if err != nil {
		return out
	}
	r.portalMu.Lock()
	defer r.portalMu.Unlock()
	for _, selected := range cfg.Portal.Selected {
		peer, err := store.Get(selected.Peer)
		if err != nil || !peer.Approved {
			continue
		}
		cached, ok := r.portalCache[selected.Peer]
		if !ok || cached.Error != "" || cached.Endpoint != peer.Endpoint || time.Since(cached.Checked) > 3*time.Minute {
			continue
		}
		for _, entry := range cached.Catalog.Services {
			if entry.ID == selected.ID {
				entry.Gateway = peer.Name + " · " + cached.Catalog.Origin
				out = append(out, entry)
			}
		}
	}
	return out
}
func (r *gatewayRuntime) servePortal(w http.ResponseWriter, req *http.Request, snap *gatewaySnapshot) bool {
	if req.URL.Path == "/pki/v1/public-ca" {
		cfg := r.currentConfig()
		if cfg.AdminDomainMode == "off" || (cfg.AdminDomainMode == "tailnet" && req.Context().Value(ingressContextKey{}) != true) {
			http.NotFound(w, req)
			return true
		}
		if site := r.adminWeb.Load(); site != nil {
			site.Handler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
		return true
	}
	if req.Method != "GET" && req.Method != "HEAD" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	cfg := r.currentConfig()
	if req.URL.Path == portal.CatalogPath {
		if !cfg.Portal.Publish {
			http.NotFound(w, req)
			return true
		}
		port := ""
		if _, p, e := net.SplitHostPort(req.Host); e == nil {
			port = p
		}
		origin := "https://" + req.Host
		catalog := portal.Catalog{Protocol: "headscale-gateway", Version: 1, Gateway: cfg.Portal.Title, Origin: origin, Services: localPortalEntries(cfg, snap.domain, port, true)}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(catalog)
		return true
	}
	if !cfg.Portal.Enabled {
		http.NotFound(w, req)
		return true
	}
	switch req.URL.Path {
	case "/portal/logo.png", "/portal/favicon.png", "/favicon.ico":
		value := cfg.Portal.Logo
		if req.URL.Path != "/portal/logo.png" {
			value = cfg.Portal.Favicon
		}
		raw, e := portal.Image(value)
		if e != nil || len(raw) == 0 {
			http.NotFound(w, req)
			return true
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(raw)
		return true
	case "/":
		port := ""
		if _, p, e := net.SplitHostPort(req.Host); e == nil {
			port = p
		}
		entries := append(localPortalEntries(cfg, snap.domain, port, false), r.remotePortalEntries(cfg)...)
		admin := ""
		if cfg.Portal.ShowAdmin && cfg.AdminEnabled && cfg.AdminDomainMode != "off" {
			host := cfg.AdminSubdomain + "." + snap.domain
			if port != "" && port != "443" {
				host = net.JoinHostPort(host, port)
			}
			admin = (&url.URL{Scheme: "https", Host: host, Path: "/admin/"}).String()
		}
		_ = portal.Render(w, req, cfg.Portal, entries, admin)
		return true
	default:
		http.NotFound(w, req)
		return true
	}
}
