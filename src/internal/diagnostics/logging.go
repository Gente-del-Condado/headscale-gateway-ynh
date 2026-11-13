// El paquete diagnostics ofrece registros privados con rotación por tamaño,
// conservando consola/journald y las llamadas estándar de registro de Go.
package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	maxBytes int64
	backups  int
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.maxBytes && w.size > 0 {
		if e := w.rotate(); e != nil {
			return 0, e
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", w.path, w.backups))
	for n := w.backups - 1; n >= 1; n-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", w.path, n), fmt.Sprintf("%s.%d", w.path, n+1))
	}
	_ = os.Rename(w.path, w.path+".1")
	return w.reopen()
}
func (w *rotatingWriter) reopen() error {
	// A configured log file must never follow a pre-existing symlink.
	if existing, err := os.Lstat(w.path); err == nil {
		if !existing.Mode().IsRegular() {
			return fmt.Errorf("ruta de log no es un archivo regular: %s", w.path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = stat.Size()
	return nil
}

// Setup conserva la interfaz original y habilita tres canales de registro.
func Setup(path string, maxMB, backups int) (func() error, error) {
	if err := Configure(filepath.Dir(path), maxMB, backups); err != nil {
		return nil, err
	}
	if filepath.Base(path) != "gateway.log" {
		custom := &rotatingWriter{path: path, maxBytes: int64(maxMB) * 1024 * 1024, backups: backups}
		if err := custom.reopen(); err != nil {
			_ = Close()
			return nil, err
		}
		_ = channels["gateway"].swap(custom)
	}
	return Close, nil
}

// truncate requiere w.mu. Windows no permite truncar un descriptor de solo anexado.
// Abre uno temporal de escritura y comprueba que sigue siendo el registro activo;
// si la ruta ha cambiado, nunca crea ni trunca un archivo de sustitución.
func (w *rotatingWriter) truncate() error {
	f, err := os.OpenFile(w.path, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	active, err := w.file.Stat()
	if err != nil {
		return err
	}
	target, err := f.Stat()
	if err != nil {
		return err
	}
	if !target.Mode().IsRegular() || !os.SameFile(active, target) {
		return fmt.Errorf("el archivo de log ha cambiado: %s", w.path)
	}
	return f.Truncate(0)
}
