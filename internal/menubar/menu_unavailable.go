//go:build !darwin || !cgo

package menubar

import "context"

const Available = false

func Run(context.Context, Settings, bool, func(Settings) error, func(string)) error {
	return ErrUnavailable
}
