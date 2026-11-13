// ca-helper instala o elimina una CA pública una vez, con elevación interactiva de permisos.
package main

import (
	"context"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"os"
	"runtime"
	"time"
)

func main() {
	if runtime.GOOS != "windows" {
		os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
	}
	action, a, err := pki.HelperRequest(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if action == "install" {
		err = pki.Install(ctx, a)
	} else {
		err = pki.Uninstall(ctx, a)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("CA:", action, a.Fingerprint)
}
