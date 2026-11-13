// Package firewall diagnostica el cortafuegos del equipo donde se ejecuta
// Gateway. Nunca abre puertos sin consentimiento, ni eleva privilegios.
package firewall

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

type Report struct {
	Platform     string   `json:"platform"`
	Listener     string   `json:"listener"`
	Port         int      `json:"port"`
	Remote       bool     `json:"remote"`
	Status       string   `json:"status"` // local, permitido, sin_regla, desconocido
	Tool         string   `json:"tool"`
	Note         string   `json:"note"`
	Instructions []string `json:"instructions"`
}

// Inspect identifica un puerto LAN legítimo. No abre puertos de la tailnet
// virtual ni recomienda exponer el API HTTP local de recuperación.
func Inspect(ctx context.Context, listen string) (Report, error) {
	host, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return Report{}, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return Report{}, errors.New("puerto de escucha inválido")
	}
	r := Report{Listener: listen, Port: port, Remote: true, Status: "desconocido"}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		r.Remote = false
		r.Status = "local"
		r.Note = "Solo loopback: no necesita una regla de entrada"
		return r, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return inspect(ctx, r), nil
}
