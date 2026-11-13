package main

import (
	"net/url"
	"strings"
)

func validGatewayLoginURL(controlURL, candidate string) bool {
	base, errBase := url.Parse(controlURL)
	target, errTarget := url.Parse(candidate)
	return errBase == nil && errTarget == nil &&
		base.Scheme == target.Scheme && base.Host == target.Host &&
		target.Scheme == "https" && strings.HasPrefix(target.Path, "/register/") &&
		target.User == nil && target.Fragment == ""
}

// Extrae únicamente enlaces de registro de nuestro propio Headscale,
// nunca URLs de terceros que aparezcan accidentalmente en un log.
func gatewayLoginURLFromLog(controlURL, line string) string {
	for _, part := range strings.Fields(line) {
		candidate := strings.Trim(part, "<>.,;()\"' ")
		if validGatewayLoginURL(controlURL, candidate) {
			return candidate
		}
	}
	return ""
}
