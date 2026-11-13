//go:build linux || darwin

package main

import (
	"os"
	"syscall"
)

func replaceGatewayProcess() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(executable, append([]string{executable}, os.Args[1:]...), os.Environ())
}

// En Unix syscall.Exec sustituye el proceso sin lanzar un segundo ejecutable.
func waitForPreviousGateway(pid int) error { return nil }
