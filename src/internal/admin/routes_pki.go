package admin

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// caDataDirectory reutiliza el data_dir existente del Gateway.
// Aquí nunca se carga, sirve ni transfiere la clave privada de la CA local.
func (s *Server) caDataDirectory() string {
	if ctl, ok := s.controller.(interface{ BackupDataDir() string }); ok {
		return ctl.BackupDataDir()
	}
	return ""
}

func (s *Server) localRoots(ctx context.Context) ([]pki.Anchor, error) {
	dir := s.caDataDirectory()
	if dir == "" {
		return nil, errors.New("data_dir no disponible")
	}
	names, e := os.ReadDir(filepath.Join(dir, "local-ca"))
	if os.IsNotExist(e) {
		return []pki.Anchor{}, nil
	}
	if e != nil {
		return nil, e
	}
	roots := []pki.Anchor{}
	for _, item := range names {
		if !item.IsDir() || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		source := filepath.Join(dir, "local-ca", item.Name(), "ca.crt")
		content, e := os.ReadFile(source)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		anchor, e := pki.ParseRoot(content)
		if e != nil {
			return nil, fmt.Errorf("CA %s: %w", item.Name(), e)
		}
		anchor.Name = item.Name()
		roots = append(roots, anchor)
	}
	return roots, nil
}
func (s *Server) activeRoot(ctx context.Context) (pki.Anchor, error) {
	roots, e := s.localRoots(ctx)
	if e != nil {
		return pki.Anchor{}, e
	}
	if len(roots) == 0 {
		return pki.Anchor{}, errors.New("la CA local aún no se ha creado")
	}
	status, e := s.controller.Status(ctx)
	if e == nil {
		if fields, ok := status.(map[string]any); ok {
			if domain, ok := fields["domain"].(string); ok && domain != "" {
				for _, ca := range roots {
					if ca.Name == domain {
						return ca, nil
					}
				}
			}
		}
	}
	if len(roots) == 1 {
		return roots[0], nil
	}
	return pki.Anchor{}, errors.New("hay varias CA locales; falta identidad de nodo confirmada")
}

type localLeafCertificate struct {
	Domain      string    `json:"domain"`
	Subject     string    `json:"subject"`
	DNSNames    []string  `json:"dns_names"`
	IPAddresses []string  `json:"ip_addresses"`
	Issuer      string    `json:"issuer"`
	Serial      string    `json:"serial"`
	Fingerprint string    `json:"fingerprint"`
	Expires     time.Time `json:"expires"`
	Expired     bool      `json:"expired"`
}

func (s *Server) localLeafInventory() ([]localLeafCertificate, error) {
	dir := s.caDataDirectory()
	if dir == "" {
		return nil, errors.New("data_dir no disponible")
	}
	names, e := os.ReadDir(filepath.Join(dir, "local-ca"))
	if os.IsNotExist(e) {
		return []localLeafCertificate{}, nil
	}
	if e != nil {
		return nil, e
	}
	found := []localLeafCertificate{}
	for _, entry := range names {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		content, e := os.ReadFile(filepath.Join(dir, "local-ca", entry.Name(), "certificate.crt"))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if len(content) > 64<<10 {
			return nil, fmt.Errorf("certificado demasiado grande: %s", entry.Name())
		}
		block, _ := pem.Decode(content)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("certificado inválido: %s", entry.Name())
		}
		cert, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			return nil, e
		}
		ips := []string{}
		for _, ip := range cert.IPAddresses {
			ips = append(ips, ip.String())
		}
		sum := sha256.Sum256(cert.Raw)
		found = append(found, localLeafCertificate{Domain: entry.Name(), Subject: cert.Subject.String(), DNSNames: cert.DNSNames, IPAddresses: ips, Issuer: cert.Issuer.String(), Serial: cert.SerialNumber.Text(16), Fingerprint: hex.EncodeToString(sum[:]), Expires: cert.NotAfter.UTC(), Expired: time.Now().After(cert.NotAfter)})
	}
	return found, nil
}

func (s *Server) pkiStore() (*pki.Store, error) { return pki.NewStore(s.caDataDirectory()) }
func (s *Server) requirePKISession(w http.ResponseWriter, r *http.Request) bool {
	if s.auth == nil || !s.auth.sessionAllowed(r) {
		problem(w, 403, "esta operación de confianza requiere sesión interactiva de administrador y CSRF")
		return false
	}
	sess, ok := s.auth.Session(r)
	if !ok || sess.role != "admin" {
		problem(w, 403, "se requiere administrador")
		return false
	}
	return true
}
func (s *Server) registerPKIRoutes(mux routeRegistrar) {
	s.registerCAHelperRoutes(mux)
	// La publicación de certificados públicos está desactivada inicialmente.
	// Nunca publica ca.key, certificate.key, metadatos de nodos ni estado administrativo.
	mux.HandleFunc("GET /pki/v1/public-ca", func(w http.ResponseWriter, r *http.Request) {
		var enabled bool
		if s.database == nil {
			http.NotFound(w, r)
			return
		}
		found, e := s.database.GetJSON("pki.public_ca_enabled_v1", &enabled)
		if e != nil || !found || !enabled {
			http.NotFound(w, r)
			return
		}
		ca, e := s.activeRoot(r.Context())
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("X-HGW-CA-SHA256", ca.Fingerprint)
		_, _ = w.Write([]byte(ca.PEM))
	})
	gate := func(fn http.HandlerFunc) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) {
				problem(w, 403, "reservado a administración")
				return
			}
			fn(w, r)
		})
	}
	mux.HandleFunc("GET /api/v1/pki", gate(func(w http.ResponseWriter, r *http.Request) {
		roots, e := s.localRoots(r.Context())
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		leaves, e := s.localLeafInventory()
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peers, e := store.List()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		var public bool
		if s.database != nil {
			_, _ = s.database.GetJSON("pki.public_ca_enabled_v1", &public)
		}
		reply(w, 200, map[string]any{"local_ca": roots, "issued_certificates": leaves, "peers": peers, "platform": pki.SystemPlatform(), "publish_public_ca": public, "auto_approve": false, "helper": pki.HelperStatus(), "helper_enabled": s.helperEnabled()}, nil)
	}))

	mux.HandleFunc("GET /api/v1/pki/discover", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		if ctl, ok := s.controller.(interface {
			PKICandidates(context.Context) (any, error)
		}); ok {
			result, err := ctl.PKICandidates(r.Context())
			reply(w, 200, result, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		lan, lanErr := pki.DiscoverLAN(ctx)
		// La API de red existente ya expone el inventario Headscale. Solo enumera
		// nodos candidatos: aparecer en él nunca implica confianza en su CA.
		var tailnet []network.Node
		if nc, ok := s.controller.(NetworkController); ok {
			if result, e := nc.NetworkNodes(ctx); e == nil {
				if inventory, ok := result.(network.Inventory); ok {
					tailnet = inventory.Nodes
				}
			}
		}
		if tailnet == nil {
			tailnet = []network.Node{}
		}
		note := "LAN mDNS/UDP: candidatos sin verificar. UDP 47991 requiere permiso del firewall privado y publicación HTTPS de la CA; comprueba siempre SHA-256."
		if lanErr != nil {
			note = lanErr.Error()
		}
		reply(w, 200, map[string]any{"lan": lan, "tailnet_candidates": tailnet, "note": note}, nil)
	}))

	mux.HandleFunc("POST /api/v1/pki/discover/download", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if !decode(w, r, &body) {
			return
		}
		ctl, ok := s.controller.(interface {
			PKIDownload(context.Context, string) (any, error)
		})
		if !ok {
			problem(w, 501, "descarga de CA no disponible")
			return
		}
		result, err := ctl.PKIDownload(r.Context(), body.Endpoint)
		reply(w, 200, result, err)
	}))
	mux.HandleFunc("PUT /api/v1/pki/publication", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		if e := s.database.PutJSON("pki.public_ca_enabled_v1", body.Enabled); e != nil {
			problem(w, 503, e.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-001", "publicación de CA pública: %t", body.Enabled)
		reply(w, 200, map[string]bool{"enabled": body.Enabled}, nil)
	}))
	mux.HandleFunc("POST /api/v1/pki/peers", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Name     string `json:"name"`
			PEM      string `json:"pem"`
			Endpoint string `json:"endpoint"`
		}
		if !decodeLimit(w, r, &body, 80<<10) {
			return
		}
		anchor, e := pki.ParseRoot([]byte(body.PEM))
		if e != nil {
			problem(w, 400, e.Error())
			return
		}
		if body.Endpoint != "" {
			if _, e = pki.PeerURL(body.Endpoint); e != nil {
				problem(w, 400, e.Error())
				return
			}
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		if _, e = store.Get(anchor.Fingerprint); e == nil {
			problem(w, 409, "la CA ya está registrada; no se sobrescribe su autorización")
			return
		}
		if !os.IsNotExist(e) {
			problem(w, 500, e.Error())
			return
		}
		anchor.Name = body.Name
		anchor.Endpoint = body.Endpoint
		anchor.Approved = false
		if e = store.Put(anchor); e != nil {
			problem(w, 400, e.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-002", "CA pública recibida, pendiente de verificar: %s", anchor.Fingerprint)
		reply(w, 201, anchor, nil)
	}))
	mux.HandleFunc("POST /api/v1/pki/peers/{fingerprint}/approve", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(w, r, &body) {
			return
		}
		fp := r.PathValue("fingerprint")
		if fp != body.Fingerprint {
			problem(w, 400, "compara e introduce la huella SHA-256 completa antes de aprobar")
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peer, e := store.Get(fp)
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		peer.Approved = true
		if e = store.Put(peer); e != nil {
			problem(w, 500, e.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-003", "CA aprobada: %s", fp)
		reply(w, 200, peer, nil)
	}))
	mux.HandleFunc("POST /api/v1/pki/peers/{fingerprint}/sync", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peer, e := store.Get(r.PathValue("fingerprint"))
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		next, e := s.syncPeer(r.Context(), peer)
		if e != nil {
			problem(w, 409, e.Error())
			return
		}
		if e = store.MarkVerified(peer.Fingerprint, peer.Endpoint, next.LastSync); e != nil {
			problem(w, 409, e.Error())
			return
		}
		reply(w, 200, map[string]any{"verified": true, "last_sync": next.LastSync}, nil)
	}))
	mux.HandleFunc("POST /api/v1/pki/peers/{fingerprint}/install", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(w, r, &body) {
			return
		}
		fp := r.PathValue("fingerprint")
		if fp != body.Fingerprint {
			problem(w, 400, "confirma la huella SHA-256 completa")
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peer, e := store.Get(fp)
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		if !peer.Approved {
			problem(w, 403, "CA sin aprobar")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
		defer cancel()
		if e = s.changeSystemCA(ctx, "install", peer); e != nil {
			problem(w, 409, e.Error()+". "+pki.Instructions(peer))
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-004", "CA instalada en confianza del sistema: %s", fp)
		reply(w, 200, map[string]any{"installed": true, "restart_recommended": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/pki/peers/{fingerprint}/uninstall", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(w, r, &body) {
			return
		}
		fp := r.PathValue("fingerprint")
		if fp != body.Fingerprint {
			problem(w, 400, "confirma la huella SHA-256 completa")
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peer, e := store.Get(fp)
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
		defer cancel()
		if e = s.changeSystemCA(ctx, "uninstall", peer); e != nil {
			problem(w, 409, e.Error()+". "+pki.RemovalInstructions(peer))
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-005", "CA retirada del almacén del sistema: %s", fp)
		reply(w, 200, map[string]any{"uninstalled": true, "restart_recommended": true}, nil)
	}))
	mux.HandleFunc("DELETE /api/v1/pki/peers/{fingerprint}", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		fp := r.PathValue("fingerprint")
		peer, e := store.Get(fp)
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		// No elimina una CA del inventario sin una decisión separada de desinstalarla
		// del SO. Si tenía confianza global, debe desinstalarse primero.
		if peer.Approved {
			peer.Approved = false
			if e = store.Put(peer); e != nil {
				problem(w, 500, e.Error())
				return
			}
			reply(w, 200, map[string]any{"revoked": true, "note": "La autorización se retiró. Si estaba instalada en el sistema, pulsa Retirar del SO por separado."}, nil)
			return
		}
		if e = store.Delete(fp); e != nil {
			problem(w, 500, e.Error())
			return
		}
		reply(w, 200, map[string]bool{"deleted": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/pki/peers/{fingerprint}/certificate", gate(func(w http.ResponseWriter, r *http.Request) {
		store, e := s.pkiStore()
		if e != nil {
			problem(w, 503, e.Error())
			return
		}
		peer, e := store.Get(r.PathValue("fingerprint"))
		if e != nil {
			problem(w, 404, e.Error())
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=ca-"+peer.Fingerprint+".crt")
		_, _ = w.Write([]byte(peer.PEM))
	}))

	mux.HandleFunc("POST /api/v1/pki/ca/{fingerprint}/install", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(w, r, &body) {
			return
		}
		fp := r.PathValue("fingerprint")
		if fp != body.Fingerprint {
			problem(w, 400, "confirma la huella SHA-256 completa")
			return
		}
		roots, e := s.localRoots(r.Context())
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		for _, root := range roots {
			if root.Fingerprint == fp {
				root.Approved = true // la CA ya es propiedad de este Gateway, pero requiere confirmación interactiva
				ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
				defer cancel()
				if e = s.changeSystemCA(ctx, "install", root); e != nil {
					problem(w, 409, e.Error()+". "+pki.Instructions(root))
					return
				}
				diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-LOCAL-TRUST", "CA propia instalada en el SO: %s", fp)
				reply(w, 200, map[string]bool{"installed": true, "restart_recommended": true}, nil)
				return
			}
		}
		problem(w, 404, "CA local no encontrada")
	}))
	mux.HandleFunc("POST /api/v1/pki/ca/{fingerprint}/uninstall", gate(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var body struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(w, r, &body) {
			return
		}
		fp := r.PathValue("fingerprint")
		if fp != body.Fingerprint {
			problem(w, 400, "confirma la huella SHA-256 completa")
			return
		}
		roots, e := s.localRoots(r.Context())
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		for _, root := range roots {
			if root.Fingerprint == fp {
				if remover, ok := s.controller.(interface{ RemoveLocalCA(string, string) error }); ok {
					e = remover.RemoveLocalCA(root.Name, fp)
				} else {
					e = pki.RemoveLocal(s.caDataDirectory(), root.Name, fp)
				}
				if e != nil {
					problem(w, 500, e.Error())
					return
				}
				diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-LOCAL-REMOVE", "CA retirada del almacén local: %s", fp)
				ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
				defer cancel()
				if e = s.changeSystemCA(ctx, "uninstall", root); e != nil {
					reply(w, 200, map[string]any{"local_removed": true, "uninstalled": false, "restart_recommended": true, "warning": e.Error(), "instructions": pki.RemovalInstructions(root)}, nil)
					return
				}
				diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-PKI-LOCAL-TRUST", "CA propia retirada del SO: %s", fp)
				reply(w, 200, map[string]bool{"local_removed": true, "uninstalled": true, "restart_recommended": true}, nil)
				return
			}
		}
		problem(w, 404, "CA local no encontrada")
	}))

	mux.HandleFunc("GET /api/v1/pki/ca/{fingerprint}/certificate", gate(func(w http.ResponseWriter, r *http.Request) {
		roots, e := s.localRoots(r.Context())
		if e != nil {
			problem(w, 500, e.Error())
			return
		}
		for _, root := range roots {
			if root.Fingerprint == r.PathValue("fingerprint") {
				w.Header().Set("Content-Type", "application/x-pem-file")
				w.Header().Set("Content-Disposition", "attachment; filename=local-ca-"+root.Fingerprint+".crt")
				_, _ = w.Write([]byte(root.PEM))
				return
			}
		}
		problem(w, 404, "CA no encontrada")
	}))
}

// La sincronización solo actualiza fechas tras verificar la identidad fijada.
// Una CA remota cambiada sigue sin confianza hasta aprobarla por un canal independiente.
func (s *Server) RunPKISynchronizer(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store, e := s.pkiStore()
			if e != nil {
				continue
			}
			peers, e := store.List()
			if e != nil {
				continue
			}
			for _, peer := range peers {
				if !peer.Approved || peer.Endpoint == "" {
					continue
				}
				next, e := s.syncPeer(ctx, peer)
				if e != nil {
					diagnostics.Emit("admin", diagnostics.Warn, "HGW-PKI-SYNC", "Sincronización pendiente para peer=%s: %v", peer.Fingerprint, e)
					continue
				}
				if e = store.MarkVerified(peer.Fingerprint, peer.Endpoint, next.LastSync); e != nil {
					diagnostics.Emit("admin", diagnostics.Warn, "HGW-PKI-SYNC", "No se pudo guardar el estado peer=%s: %v", peer.Fingerprint, e)
				}
			}
		}
	}
}

func (s *Server) syncPeer(ctx context.Context, peer pki.Anchor) (pki.Anchor, error) {
	if ctl, ok := s.controller.(interface {
		PKISync(context.Context, pki.Anchor) (pki.Anchor, error)
	}); ok {
		return ctl.PKISync(ctx, peer)
	}
	return pki.Sync(ctx, peer)
}
