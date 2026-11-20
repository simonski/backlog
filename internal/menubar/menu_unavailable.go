//go:build !darwin || !cgo

package menubar

import "context"

const Available = false

func Run(context.Context, string) error {
	return ErrUnavailable
}
