//go:build !linux || android

package pki

import "context"

// Windows/macOS utilizan el descubrimiento UDP incluido; no requieren Avahi.
func DiscoverLAN(ctx context.Context, ports ...int) ([]Service, error) {
	return DiscoverPortable(ctx, ports...)
}
