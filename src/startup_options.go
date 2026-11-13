package main

// Todos los instaladores usan el mismo contrato de arranque. SQLite conserva
// la configuración; el archivo aporta valores iniciales y opciones de conexión HTTP.
import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

type startupOptions struct {
	admin.HTTPOptions
	ListenAddr       string `json:"listen_addr"`
	NodeName         string `json:"node_name"`
	LANListenAddr    string `json:"lan_listen_addr"`
	InitialAdminFile string `json:"initial_admin_file"`
}

func readPrivateJSON(path string, dst any) error {
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || (runtime.GOOS != "windows" && stat.Mode().Perm()&0077 != 0) || stat.Size() > 16<<10 {
		return errors.New("startup JSON must be a private regular file under 16 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected startup JSON data")
	}
	return nil
}

func loadStartupOptions() (*startupOptions, error) {
	path := os.Getenv("GATEWAY_BOOTSTRAP_FILE")
	if path == "" {
		return nil, nil
	}
	var d startupOptions
	if err := readPrivateJSON(path, &d); err != nil {
		return nil, err
	}
	if err := d.HTTPOptions.Validate(); err != nil {
		return nil, err
	}
	if err := admin.ValidateHTTPListenAddress(d.ListenAddr, d.HTTPOptions); err != nil {
		return nil, err
	}
	if d.NodeName != "" && (len(d.NodeName) > 63 || normalizeDNSName(d.NodeName) != d.NodeName) {
		return nil, errors.New("invalid stable node name")
	}
	return &d, nil
}

func initializeAdministrator(db *state.Store, auth *admin.AuthManager, d *startupOptions) error {
	if d == nil || d.InitialAdminFile == "" {
		return nil
	}
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readPrivateJSON(d.InitialAdminFile, &credentials); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer os.Remove(d.InitialAdminFile)
	var initialized bool
	_, err := db.GetJSON("deployment.admin_initialized.v1", &initialized)
	if err != nil {
		return err
	}
	// A completed installation or configured identity always wins over an
	// installer password. Upgrades must never reset authentication or MFA.
	if initialized || auth.Current().Mode != "token" {
		return nil
	}
	if strings.TrimSpace(credentials.Username) == "" || len(credentials.Password) < 12 || len(credentials.Password) > 72 {
		return errors.New("initial administrator needs a username and a 12-72 byte password")
	}
	if err := auth.Update(admin.AuthUpdate{Mode: "local", LocalUsername: credentials.Username, LocalPassword: credentials.Password}); err != nil {
		return err
	}
	if err := db.PutJSON("deployment.admin_initialized.v1", true); err != nil {
		return err
	}
	return db.PutJSON("recovery_bootstrap_pending_v1", false)
}

func startupInitialSettings(settings map[string]string) error {
	d, err := loadStartupOptions()
	if err != nil {
		return fmt.Errorf("startup: %w", err)
	}
	if d == nil {
		return nil
	}
	// Los valores persistidos siguen siendo portables. La dirección HTTP asignada
	// por el anfitrión es una opción del proceso, no otra configuración persistente.
	if d.LANListenAddr != "" {
		settings["server.listen_addr"] = d.LANListenAddr
	}
	return nil
}

// Pin identity on every installation. Machine names, paths and installers can
// change without registering another VPN node. Adopt one existing state directory.
func stateHostname(dbPath string, d *startupOptions, ordinary string) (string, error) {
	db, err := openCatalog(dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var name string
	found, err := db.GetJSON("deployment.state_hostname.v1", &name)
	if err != nil {
		return "", err
	}
	if found {
		if name == "" || normalizeDNSName(name) != name {
			return "", errors.New("invalid stored node name")
		}
		return name, nil
	}
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "tailscale-state-*"))
	if err != nil {
		return "", err
	}
	var existing []string
	for _, entry := range entries {
		info, err := os.Lstat(entry)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			continue
		}
		candidate := strings.TrimPrefix(filepath.Base(entry), "tailscale-state-")
		if candidate != "" && normalizeDNSName(candidate) == candidate {
			existing = append(existing, candidate)
		}
	}
	name = ordinary
	if d != nil && d.NodeName != "" {
		name = d.NodeName
	}
	if len(existing) == 1 {
		name = existing[0]
	} else if len(existing) > 1 {
		matched := false
		for _, candidate := range existing {
			if candidate == name {
				matched = true
				break
			}
		}
		if !matched {
			return "", errors.New("multiple tsnet state directories; select the original identity before migration")
		}
	}
	if err := db.PutJSON("deployment.state_hostname.v1", name); err != nil {
		return "", err
	}
	return name, nil
}
