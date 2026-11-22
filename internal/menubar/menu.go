package menubar

import "errors"

var ErrUnavailable = errors.New("the menu bar requires a macOS build with cgo enabled")

type Settings struct {
	DashboardURL string `json:"dashboard_url"`
	Username     string `json:"username"`
}
