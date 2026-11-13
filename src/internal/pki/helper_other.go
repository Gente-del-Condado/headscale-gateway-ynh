//go:build android || (!linux && !windows && !darwin)

package pki

import (
	"context"
	"errors"
)

func elevateHelper(context.Context, string, []string) error {
	return errors.New("helper no disponible en este sistema")
}
