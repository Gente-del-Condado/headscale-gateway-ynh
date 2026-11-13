//go:build !android

package main

// Las plataformas existentes conservan sus listeners y su ciclo de vida.
func startMobileServices(_ *gatewayRuntime, _ *vpnState) error { return nil }
