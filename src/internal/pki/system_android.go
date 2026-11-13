//go:build android

package pki

import (
	"context"
	"errors"
)

func SystemPlatform() string { return "android" }
func Instructions(a Anchor) string {
	return "Android: descarga la CA pública .crt y abre Ajustes > Seguridad > Cifrado y credenciales > Instalar certificado > CA. Los nombres pueden variar según el dispositivo. Comprueba antes la huella SHA-256: " + a.Fingerprint
}
func RemovalInstructions(a Anchor) string {
	return "Android: retira manualmente la CA de las credenciales de usuario en Ajustes > Seguridad. Comprueba su huella SHA-256: " + a.Fingerprint
}
func Install(context.Context, Anchor) error {
	return errors.New("Android requiere instalar la CA pública desde sus ajustes de seguridad con consentimiento del usuario")
}
func Uninstall(context.Context, Anchor) error {
	return errors.New("Android requiere retirar la CA desde las credenciales de usuario en sus ajustes de seguridad")
}
