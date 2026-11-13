//go:build windows

package main

import (
	"context"
	"errors"
)

func (a *adminController) ScheduleBackupRestore(context.Context, string, string) (string, error) {
	return "", errors.New("en Windows, para evitar reemplazar SQLite abierto, detén el Gateway y restaura desde PowerShell con --restore-backup; la restauración web automática requiere un servicio auxiliar de reinicio")
}
func applyStagedRestore(string, string) error {
	return errors.New("restauración web offline no disponible en Windows; usa --restore-backup con Gateway detenido")
}
func restartAfterStagedRestore() error {
	return errors.New("restauración web offline no disponible en Windows; usa --restore-backup con Gateway detenido")
}
