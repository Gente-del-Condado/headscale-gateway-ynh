// Package cli implementa diagnósticos sin abrir SQLite en escritura ni arrancar
// la VPN. Es apto para entornos Linux, Windows y macOS.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
)

var Version = "development"

func IsCommand(s string) bool { return s == "logs" || s == "doctor" || s == "version" }

func Run(args []string) error { return Execute(args, os.Stdout) }

// Execute permite verificar el CLI sin crear interfaces de red.
func Execute(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("falta comando: logs|doctor|version")
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(out, i18n.T("Headscale Gateway %s - Go %s - %s/%s\n"), Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	case "doctor":
		opts, err := parseOptions("doctor", args[1:])
		if err != nil {
			return err
		}
		return doctor(out, opts)
	case "logs":
		if len(args) < 2 {
			return errors.New("uso: logs status|tail|follow [--channel ...] [--lines ...]")
		}
		op := args[1]
		if op != "status" && op != "tail" && op != "follow" {
			return fmt.Errorf("subcomando logs desconocido: %s", op)
		}
		opts, err := parseOptions("logs "+op, args[2:])
		if err != nil {
			return err
		}
		directory, err := opts.logDirectory()
		if err != nil {
			return err
		}
		switch op {
		case "status":
			return status(out, directory)
		case "tail", "follow":
			rows, err := diagnostics.TailAt(directory, opts.channel, opts.lines)
			if err != nil {
				return err
			}
			for _, row := range rows {
				fmt.Fprintln(out, row)
			}
			if op == "follow" {
				return Follow(out, filepath.Join(directory, opts.channel+".log"), time.Second)
			}
		}
		return nil
	default:
		return fmt.Errorf("comando desconocido %q", args[0])
	}
}

type options struct {
	db, dir, channel string
	lines            int
}

func parseOptions(name string, args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.db, "db", "", "Ruta de gateway.sqlite")
	fs.StringVar(&o.dir, "dir", "", "Directorio de logs; prevalece sobre --db")
	fs.StringVar(&o.channel, "channel", "gateway", "gateway|proxy|admin")
	fs.IntVar(&o.lines, "lines", 50, "últimas 1-200 líneas")
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("opciones CLI inválidas: %w", err)
	}
	if len(fs.Args()) > 0 {
		return o, fmt.Errorf("argumentos inesperados: %s", strings.Join(fs.Args(), " "))
	}
	if o.lines < 1 || o.lines > 200 {
		return o, errors.New("--lines debe estar entre 1 y 200")
	}
	if o.channel != "gateway" && o.channel != "proxy" && o.channel != "admin" {
		return o, errors.New("--channel debe ser gateway, proxy o admin")
	}
	return o, nil
}
func (o options) logDirectory() (string, error) {
	if o.dir != "" {
		if err := diagnostics.ValidateDir(o.dir); err != nil {
			return "", err
		}
		return filepath.Clean(o.dir), nil
	}
	db, err := o.databasePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(db), "logs"), nil
}
func (o options) databasePath() (string, error) {
	if o.db != "" {
		return filepath.Abs(o.db)
	}
	if runtime.GOOS == "windows" {
		if root := os.Getenv("ProgramData"); root != "" {
			legacy := filepath.Join(root, "HeadscaleGateway", "gateway.sqlite")
			if _, err := os.Stat(legacy); err == nil {
				return legacy, nil
			}
		}
		if root := os.Getenv("LOCALAPPDATA"); root != "" {
			return filepath.Join(root, "HeadscaleGateway", "gateway.sqlite"), nil
		}
	}
	if runtime.GOOS == "linux" {
		if os.Getenv("USER") == "root" {
			return "/var/lib/headscale-gateway/gateway.sqlite", nil
		}
		if root := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(root) {
			return filepath.Join(root, "headscale-gateway", "gateway.sqlite"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local/share/headscale-gateway/gateway.sqlite"), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "HeadscaleGateway", "gateway.sqlite"), nil
}
func status(out io.Writer, dir string) error {
	fmt.Fprintln(out, i18n.T("Canales de registro (solo lectura):"))
	for _, name := range []string{"gateway", "proxy", "admin"} {
		path := filepath.Join(dir, name+".log")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(out, i18n.T("%-8s no existe: %s\n"), name, path)
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			fmt.Fprintf(out, i18n.T("%-8s ERROR: no es archivo regular\n"), name)
			continue
		}
		fmt.Fprintf(out, i18n.T("%-8s %s bytes %s\n"), name, strconv.FormatInt(info.Size(), 10), path)
	}
	return nil
}
func doctor(out io.Writer, o options) error {
	db, err := o.databasePath()
	if err != nil {
		return err
	}
	dir, err := o.logDirectory()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, i18n.T("Diagnóstico Headscale Gateway (%s/%s, %s)\n"), runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(out, i18n.T("SQLite: %s\n"), db)
	if info, e := os.Lstat(db); e == nil {
		if info.Mode().IsRegular() {
			fmt.Fprintln(out, i18n.T("  [OK] Base encontrada"))
		} else {
			fmt.Fprintln(out, i18n.T("  [ADVERTENCIA] La ruta no es archivo regular"))
		}
	} else if errors.Is(e, os.ErrNotExist) {
		fmt.Fprintln(out, i18n.T("  [ADVERTENCIA] No existe aún (¿primer arranque?)"))
	} else {
		return e
	}
	fmt.Fprintf(out, i18n.T("Logs: %s\n"), dir)
	return status(out, dir)
}
