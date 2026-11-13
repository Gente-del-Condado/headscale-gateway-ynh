//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"syscall"
)

// replaceGatewayProcess inicia primero un sustituto que espera a que este
// proceso libere los sockets y SQLite. No utiliza PowerShell ni permisos de
// administrador. Para servicios Windows supervisados se recomienda que el SCM
// administre el reinicio; este modo está destinado a ejecución interactiva.
func replaceGatewayProcess() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args := append(append([]string(nil), os.Args[1:]...), fmt.Sprintf("--restart-wait-pid=%d", os.Getpid()))
	cmd := exec.Command(executable, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("no se puede iniciar el nuevo Gateway: %w", err)
	}
	if err = cmd.Process.Release(); err != nil {
		return err
	}
	// La instancia siguiente no abrirá ningún listener antes de nuestra salida.
	os.Exit(0)
	return nil
}

// waitForPreviousGateway bloquea como máximo 30 segundos; evita los conflictos
// por puertos y ficheros SQLite al reiniciar desde el panel de Windows.
func waitForPreviousGateway(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("PID de reinicio inválido")
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, syscall.Errno(87)) {
		return nil
	} // ya finalizó
	if err != nil {
		return fmt.Errorf("no se puede comprobar el Gateway anterior: %w", err)
	}
	defer syscall.CloseHandle(h)
	code, err := syscall.WaitForSingleObject(h, uint32((30*time.Second)/time.Millisecond))
	if err != nil {
		return err
	}
	if code != syscall.WAIT_OBJECT_0 {
		return errors.New("el Gateway anterior no terminó dentro de 30 segundos")
	}
	return nil
}
