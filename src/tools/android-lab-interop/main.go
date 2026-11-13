// Comprueba con el controlador SQLite de Linux una copia creada en Android.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/configtransfer"
	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func main() {
	archive := flag.String("archive", "", "Copia cifrada Android")
	passwordFile := flag.String("password-file", "", "Archivo privado con la contraseña de laboratorio")
	profileFile := flag.String("profile", "", "Perfil .hgwcfg creado por Android")
	keyFile := flag.String("headscale-key-file", "", "Clave API temporal para comparar los secretos sin mostrarlos")
	flag.Parse()
	password, err := os.ReadFile(*passwordFile)
	if err != nil {
		panic(err)
	}
	if *profileFile != "" {
		raw, err := os.ReadFile(*profileFile)
		if err != nil {
			panic(err)
		}
		profile, err := configtransfer.Import(raw, strings.TrimSpace(string(password)))
		if err != nil || !profile.IncludesSecrets || profile.Settings["domain.base_domain"] != "lab.example.test" {
			panic("el perfil Android no se puede leer en Linux")
		}
		fmt.Println("Perfil cifrado Android leído y autenticado en Linux")
	}
	dir, err := os.MkdirTemp("", "hgw-interop-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	manifest, err := securebackup.Extract(context.Background(), *archive, strings.TrimSpace(string(password)), dir)
	if err != nil {
		panic(err)
	}
	data := filepath.Join(dir, "data")
	if err := state.ValidateBackupPair(filepath.Join(data, "gateway.sqlite"), filepath.Join(data, "master.key")); err != nil {
		panic(err)
	}
	db, err := state.Open(filepath.Join(data, "gateway.sqlite"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	services, err := db.ListServices()
	if err != nil {
		panic(err)
	}
	settings, err := db.GetSettings()
	if err != nil {
		panic(err)
	}
	if len(services) == 0 || settings["domain.base_domain"] != "lab.example.test" {
		panic("la copia no conserva los servicios y ajustes de prueba")
	}
	if *keyFile != "" {
		key, err := os.ReadFile(*keyFile)
		if err != nil || settings["headscale.api_key"] != strings.TrimSpace(string(key)) {
			panic("los secretos cifrados no se recuperan con el controlador SQLite de Linux")
		}
	}
	fmt.Printf("Copia Android descifrada, verificada y leída con SQLite en Linux: modo %s, %d servicios\n", manifest.Mode, len(services))
}
