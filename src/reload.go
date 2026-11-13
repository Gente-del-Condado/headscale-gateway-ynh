package main

import

// Las solicitudes de recarga son persistentes en SQLite, en lugar de utilizar archivos centinela en el sistema de archivos.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"

	"time"
)

func requestReload(dbPath string) error {
	db, err := openCatalog(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.PutJSON("reload_request_v1", time.Now().UTC().UnixNano())
}
func watchReloadRequests(dbPath, dataDir string, r *gatewayRuntime, vpn *vpnState, provider DNSProvider) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var last int64
	// No retransmitas solicitudes obsoletas registradas antes de que se iniciara este proceso.
	if db, err := openCatalog(dbPath); err == nil {
		_, _ = db.GetJSON("reload_request_v1", &last)
		_ = db.Close()
	}
	for range ticker.C {
		db, err := openCatalog(dbPath)
		if err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-RELOAD-001", "SQLite reload: %v", err)
			continue
		}
		var id int64
		found, err := db.GetJSON("reload_request_v1", &id)
		_ = db.Close()
		if err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-RELOAD-002", "SQLite reload: %v", err)
			continue
		}
		if !found || id == last {
			continue
		}
		last = id
		if err := r.reloadServices(dbPath, vpn, provider); err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-RELOAD-003", "recarga SQL rechazada: %v", err)
		}
	}
}

var _ = fmt.Sprintf
