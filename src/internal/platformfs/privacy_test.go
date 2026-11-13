package platformfs

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// infoSimulada permite probar los permisos de Unix y los tipos de archivo
// sin depender de chmod ni del sistema de archivos del equipo de pruebas.
type infoSimulada struct {
	modo os.FileMode
}

func (i infoSimulada) Name() string       { return "elemento" }
func (i infoSimulada) Size() int64        { return 0 }
func (i infoSimulada) Mode() os.FileMode  { return i.modo }
func (i infoSimulada) ModTime() time.Time { return time.Time{} }
func (i infoSimulada) IsDir() bool        { return i.modo.IsDir() }
func (i infoSimulada) Sys() any           { return nil }

func TestPrivateDirectory(t *testing.T) {
	// En Windows, la implementación actual solo comprueba el tipo de archivo:
	// los bits Unix no permiten determinar los permisos reales de NTFS.
	windows := runtime.GOOS == "windows"

	casos := []struct {
		nombre string
		modo   os.FileMode
		quiere bool
	}{
		{"directorio 0700", os.ModeDir | 0700, true},
		{"directorio 0750", os.ModeDir | 0750, windows},
		{"directorio 0755", os.ModeDir | 0755, windows},
		{"directorio 0777", os.ModeDir | 0777, windows},
		{"archivo 0600", 0600, false},
		{"enlace simbolico", os.ModeSymlink | 0777, false},
	}

	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			obtenido := PrivateDirectory(infoSimulada{modo: tc.modo})
			if obtenido != tc.quiere {
				t.Errorf("PrivateDirectory(%v) = %v, se esperaba %v", tc.modo, obtenido, tc.quiere)
			}
		})
	}
}

func TestPrivateFile(t *testing.T) {
	windows := runtime.GOOS == "windows"

	casos := []struct {
		nombre string
		modo   os.FileMode
		quiere bool
	}{
		{"archivo 0600", 0600, true},
		{"archivo 0640", 0640, windows},
		{"archivo 0644", 0644, windows},
		{"archivo 0666", 0666, windows},
		{"directorio 0700", os.ModeDir | 0700, false},
		{"enlace simbolico", os.ModeSymlink | 0777, false},
		{"tuberia nombrada", os.ModeNamedPipe | 0600, false},
	}

	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			obtenido := PrivateFile(infoSimulada{modo: tc.modo})
			if obtenido != tc.quiere {
				t.Errorf("PrivateFile(%v) = %v, se esperaba %v", tc.modo, obtenido, tc.quiere)
			}
		})
	}
}
