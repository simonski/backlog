//go:build !darwin || !cgo

package menubar

import (
	"context"

	"harness.engineering/internal/wire"
)

const Available = false

func Run(context.Context, Settings, bool, func(Settings) error, func(wire.ServerVersion) bool) error {
	return ErrUnavailable
}
