package main

import (
	"context"
	"errors"
	"tailscale.com/ipn"
	"time"
)

func loadVPNState(path string) (*vpnState, error) {
	db, err := openCatalog(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	v := &vpnState{}
	_, err = db.GetJSON("vpn.manual_pause.v1", &v.paused)
	return v, err
}
func (v *vpnState) isPaused() bool { v.mu.Lock(); defer v.mu.Unlock(); return v.paused }
func (a *adminController) saveVPNPause(paused bool) error {
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.PutJSON("vpn.manual_pause.v1", paused); err != nil {
		return err
	}
	a.vpn.mu.Lock()
	a.vpn.paused = paused
	if paused {
		a.vpn.loginURL = ""
	}
	a.vpn.mu.Unlock()
	return nil
}

const pauseFromVPNMessage = "No se puede pausar desde el panel conectado por la VPN del Gateway: perderías el acceso. Abre el panel local desde el equipo del Gateway o utiliza su acceso HTTPS por LAN antes de pausar."

func (a *adminController) NetworkDisconnect(ctx context.Context) (any, error) {
	// Usa el contexto confiable del listener, nunca Host ni cabeceras reenviadas.
	// Rechaza antes de guardar la pausa o tocar tsnet, incluso si se omite la interfaz.
	if ctx.Value(ingressContextKey{}) == true {
		return nil, errors.New(pauseFromVPNMessage)
	}

	a.vpn.actionMu.Lock()
	defer a.vpn.actionMu.Unlock()
	s := a.networkServer()
	if s == nil {
		return nil, errors.New("no hay una VPN activa que desconectar")
	}
	c, err := s.LocalClient()
	if err != nil {
		return nil, err
	}
	if err = a.saveVPNPause(true); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, err = c.EditPrefs(ctx, &ipn.MaskedPrefs{WantRunningSet: true, Prefs: ipn.Prefs{WantRunning: false}})
	if err != nil {
		if rollback := a.saveVPNPause(false); rollback != nil {
			return nil, errors.Join(err, rollback)
		}
		return nil, err
	}
	return map[string]any{"state": "Stopped", "message": "VPN pausada. Identidad conservada; pulsa Reconectar para reanudar."}, nil
}
