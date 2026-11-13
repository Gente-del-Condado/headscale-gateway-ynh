//go:build !windows

package main

func enableConsoleANSI() bool { return true }
