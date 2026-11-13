package cli

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"io"
	"os"
	"time"
)

// Follow observa un único archivo regular. Detecta truncamientos y cambios
// de inodo por rotación. El usuario puede cancelar con Ctrl+C.
func Follow(out io.Writer, path string, interval time.Duration) error {
	var f *os.File
	var offset int64
	var old os.FileInfo
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	for {
		stat, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				time.Sleep(interval)
				continue
			}
			return err
		}
		if !stat.Mode().IsRegular() {
			return errors.New("log no es archivo regular")
		}
		if f == nil || !os.SameFile(stat, old) || stat.Size() < offset {
			if f != nil {
				_ = f.Close()
			}
			f, err = os.Open(path)
			if err != nil {
				return err
			}
			old = stat
			offset = 0
		}
		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		// Se limita el volumen de un ciclo para no bloquear indefinidamente.
		reader := bufio.NewReader(io.LimitReader(f, 1024*1024))
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				if !errors.Is(e, io.EOF) {
					return e
				}
				// Una línea incompleta puede terminar en el siguiente ciclo.
				break
			}
			if _, err = fmt.Fprint(out, diagnostics.Redact(line)); err != nil {
				return err
			}
			offset += int64(len(line))
		}
		time.Sleep(interval)
	}
}
