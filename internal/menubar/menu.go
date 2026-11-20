package menubar

import "errors"

var ErrUnavailable = errors.New("the menu bar requires a macOS build with cgo enabled")
