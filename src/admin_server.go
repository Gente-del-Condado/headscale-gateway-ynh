package main

// Ciclo de vida del arranque y del listener HTTP para el panel administrativo.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net"
	"net/http"
	"sync"
	"time"
)

func (a *adminController) ClearProxyConnections() { clearIdleProxyConnections() }

type adminController struct {
	configPath string
	runtime    *gatewayRuntime
	vpn        *vpnState
	provider   DNSProvider
	started    time.Time
}

func startAdministration(cfg Config, dbPath string, runtime *gatewayRuntime, vpn *vpnState, provider DNSProvider) error {
	options, err := loadStartupOptions()
	if err != nil {
		return err
	}
	listenAddr := cfg.AdminListenAddr
	var httpOptions admin.HTTPOptions
	if options != nil {
		listenAddr = options.ListenAddr
		httpOptions = options.HTTPOptions
	}
	if err := admin.ValidateHTTPListenAddress(listenAddr, httpOptions); err != nil {
		return err
	}

	db, err := openCatalog(dbPath)
	if err != nil {
		return fmt.Errorf("SQLite: %w", err)
	}
	token, first, err := getOrCreateRecoveryToken(db)
	if err != nil {
		_ = db.Close()
		return err
	}
	controller := &adminController{configPath: dbPath, runtime: runtime, vpn: vpn, provider: provider, started: time.Now()}
	if err := startMobileServices(runtime, vpn); err != nil {
		_ = db.Close()
		return err
	}
	handler, err := admin.New(token, controller)
	if err != nil {
		_ = db.Close()
		return err
	}
	authManager, err := admin.NewAuthManagerSQLite(db)
	if err != nil {
		_ = db.Close()
		return err
	}
	if err := initializeAdministrator(db, authManager, options); err != nil {
		_ = db.Close()
		return err
	}
	if _, err := db.GetJSON("recovery_bootstrap_pending_v1", &first); err != nil {
		db.Close()
		return err
	}
	handler.WithAuth(authManager)
	if err = handler.WithState(db); err != nil {
		_ = db.Close()
		return err
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		_ = db.Close()
		return err
	}
	var bannerOnce sync.Once
	runtime.showAdminConsole = func() {
		bannerOnce.Do(func() {
			code := ""
			if first && adminConsoleInteractive() {
				var codeErr error
				code, codeErr = newAdminBootstrapCode()
				if codeErr != nil {
					diagnostics.Emit("admin", diagnostics.Error, "HGW-RECOVERY-CODE", "No se puede continuar: %v", codeErr)
					return
				}
				if err := db.PutJSON("recovery_bootstrap_pending_v1", false); err != nil {
					diagnostics.Emit("admin", diagnostics.Error, "HGW-RECOVERY-PENDING", "No se puede continuar: %v", err)
					return
				}
				authManager.SetBootstrap(token, code)
			} else if first {
				// Conserva la recuperación para el próximo arranque interactivo en primer plano.
				if err := db.PutJSON("recovery_bootstrap_pending_v1", true); err != nil {
					diagnostics.Emit("admin", diagnostics.Error, "HGW-RECOVERY-PENDING", "No se puede continuar: %v", err)
				}
			}
			printAdminConsole(adminConsoleURL(listener.Addr().String(), httpOptions), code, first)
		})
	}

	syncCtx, syncCancel := context.WithCancel(context.Background())
	go handler.RunPKISynchronizer(syncCtx)
	go controller.portalRefreshLoop(syncCtx)
	go controller.portalDiscoveryLoop(syncCtx, db, cfg.LocalListenAddr)
	adminHandler := handler.Handler()

	startPublicCALAN(syncCtx, cfg, runtime, adminHandler)
	runtime.adminWeb.Store(&adminSiteHandler{Handler: adminHandler})
	server := &http.Server{ErrorLog: diagnostics.HTTPErrorLogger("admin"), Handler: handler.Handler(httpOptions), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnostics.Emit("admin", diagnostics.Error, "HGW-ADMIN-SERVER-002", "panel: %v", err)
		}
	}()
	shutdownTasks.Add(1)
	go func() {
		defer shutdownTasks.Done()
		<-processContext.Done()
		syncCancel()
		shutdownHTTPServer(server)
		_ = db.Close()
	}()
	if panelURL := adminConsoleURL(listener.Addr().String(), httpOptions); panelURL != "" {
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ADMIN-SERVER-003", "Panel en %s; credencial de recuperación protegida en SQLite", panelURL)
	} else {
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ADMIN-SERVER-004", "Panel disponible mediante el proxy autorizado; escucha interna: %s", listener.Addr())
	}
	time.AfterFunc(20*time.Second, runtime.showAdminConsole)
	return nil
}

// La clave Bearer de recuperación se guarda en la base privada, no en otro
// archivo escribible. Su texto sin cifrar nunca debe llegar a registros ni API públicas.
func getOrCreateRecoveryToken(db *state.Store) (string, bool, error) {
	var token string
	found, err := db.GetJSON("recovery_token_v1", &token)
	if err != nil {
		return "", false, err
	}
	if found {
		var pending bool
		if _, err = db.GetJSON("recovery_bootstrap_pending_v1", &pending); err != nil {
			return "", false, err
		}
		raw, e := hex.DecodeString(token)
		if e != nil || len(raw) != 32 {
			return "", false, fmt.Errorf("token de recuperación almacenado inválido")
		}
		return token, pending, nil
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", false, err
	}
	token = hex.EncodeToString(raw)
	if err = db.PutJSON("recovery_token_v1", token); err != nil {
		return "", false, err
	}
	if err = db.PutJSON("recovery_bootstrap_pending_v1", true); err != nil {
		return "", false, err
	}
	return token, true, nil
}
