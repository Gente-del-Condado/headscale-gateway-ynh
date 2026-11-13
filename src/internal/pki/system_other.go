//go:build !linux && !windows && !darwin

package pki

import (
	"context"
	"errors"
)

func SystemPlatform() string { return "unsupported" }
func Instructions(a Anchor) string {
	return "Instalación manual requerida; comprueba SHA-256: " + a.Fingerprint
}
func Install(context.Context, Anchor) error {
	return errors.New("instalación automática no implementada en este sistema")
}
func Uninstall(context.Context, Anchor) error {
	return errors.New("desinstalación automática no implementada en este sistema")
}

func RemovalInstructions(a Anchor) string {
	return "Retira manualmente del SO la CA SHA-256: " + a.Fingerprint
}
