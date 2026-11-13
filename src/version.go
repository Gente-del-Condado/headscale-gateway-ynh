package main

import (
	_ "embed"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/cli"
)

//go:embed VERSION
var releaseVersion string

func init() { cli.Version = strings.TrimSpace(releaseVersion) }
