package main

import (
	"golang.org/x/sys/windows"
	"os"
)

func enableConsoleANSI() bool {
	h := windows.Handle(os.Stderr.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
