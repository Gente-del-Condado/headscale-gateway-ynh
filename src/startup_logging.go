package main

// Aplica los ajustes persistidos de rotación y nivel de logs.
// Las preferencias explícitas de CLI/entorno prevalecen sobre SQLite.
import (
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
)

func restoreLogSettings(configPath string, overridden bool) {
	// Restaura los canales y la rotación de registros guardados desde el panel.
	{
		db, openErr := openCatalog(configPath)
		if openErr == nil {
			var stored struct {
				Directory string `json:"directory"`
				MaxMB     int    `json:"max_mb"`
				Backups   int    `json:"backups"`
				Level     string `json:"level"`
			}
			has, readErr := db.GetJSON("diagnostics.logging.v1", &stored)
			_ = db.Close()
			if readErr != nil {
				diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOG-002", "leer configuración: %v", readErr)
			}
			if has && readErr == nil {
				if !overridden && stored.Level != "" {
					if e := diagnostics.SetLevel(stored.Level); e != nil {
						diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOG-004", "nivel inválido: %v", e)
					}
				}
				if e := diagnostics.Configure(stored.Directory, stored.MaxMB, stored.Backups); e != nil {
					diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOG-003", "ajustes guardados de log rechazados: %v", e)
				}
			}
		}
	}
}
