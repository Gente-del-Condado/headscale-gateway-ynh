package main

import "github.com/gentedelcondado/headscale-gateway/internal/pki"

// Serializa la eliminación con emisión y recarga; las instantáneas TLS existentes siguen válidas.
func (a *adminController) RemoveLocalCA(name, fingerprint string) error {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	return pki.RemoveLocal(a.runtime.dataDir, name, fingerprint)
}
