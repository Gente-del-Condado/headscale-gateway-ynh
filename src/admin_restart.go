package main

import (
	"context"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"sync/atomic"
	"time"
)

var restartPending atomic.Bool

// ScheduleRestart solo lo invoca la API administrativa autenticada.
// Reinicia en Linux/macOS con exec y en Windows creando una instancia
// sustituta que espera la liberación de SQLite y de los puertos.
func (a *adminController) ScheduleRestart(_ context.Context) error {
	if !restartPending.CompareAndSwap(false, true) {
		return errors.New("ya existe un reinicio programado")
	}
	go func() {
		// Permitir terminar la respuesta HTTP antes del exec.
		time.Sleep(1200 * time.Millisecond)
		if err := replaceGatewayProcess(); err != nil {
			restartPending.Store(false)
			diagnostics.Emit("admin", diagnostics.Error, "HGW-ADMIN-RESTART-001", "reiniciando Gateway: %v", err)
		}
	}()
	return nil
}
