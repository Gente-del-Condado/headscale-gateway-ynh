package main

// Rotación de claves de recuperación y arranque inicial local de un solo uso.
import (
	"crypto/rand"
	"encoding/hex"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func newAdminBootstrapCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Un operador con acceso al sistema de archivos de la base de datos privada puede rotar un token de recuperación
// perdido sin llegar a mostrarlo nunca como argumento de un comando ni como entrada en un registro.
func rotateRecoveryCredential(db *state.Store) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	if err := db.PutJSON("recovery_token_v1", hex.EncodeToString(raw)); err != nil {
		return err
	}
	return db.PutJSON("recovery_bootstrap_pending_v1", true)
}
