package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"harness.engineering/internal/local"
	"harness.engineering/internal/menubar"
	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
)

func runMenu(ctx context.Context, args []string, diagnostics io.Writer) error {
	flags := newFlags("menu", diagnostics, " [options]")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "local dashboard host:port")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if !menubar.Available {
		return menubar.ErrUnavailable
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	path, err := dashboardURLPath()
	if err != nil {
		return err
	}
	settings, err := readClientSettings()
	if err != nil {
		return fmt.Errorf("read client settings (%w)", err)
	}
	settings.Username = backlogUsername(settings.Username)
	if hasAddressOverride(flags) {
		if err := local.ValidateAddress(address); err != nil {
			return err
		}
		settings.DashboardURL = "http://" + address + "/"
	} else if settings.DashboardURL, err = dashboardURL("http://" + address + "/"); err != nil {
		return fmt.Errorf("read BACKLOG_URL (%w)", err)
	}
	initialURLValid := true
	if _, err := parseDashboardURL(settings.DashboardURL); err != nil {
		fmt.Fprintln(diagnostics, err)
		initialURLValid = false
	}

	save := func(next menubar.Settings) error {
		if _, err := parseDashboardURL(next.DashboardURL); err != nil {
			return err
		}
		if !model.ValidMonitorName(next.Username) {
			return errors.New("username must be at most 48 printable characters")
		}
		return saveClientSettings(path, next)
	}
	lastVersion := "unknown"
	versionChanged := func(serverVersion string) {
		if !version.Valid(serverVersion) {
			serverVersion = ""
		}
		if lastVersion == serverVersion {
			return
		}
		lastVersion = serverVersion
		if serverVersion == "" {
			fmt.Fprintln(diagnostics, "Backlog server version unavailable.")
		} else if warning := version.Warning(version.Client, serverVersion); warning != "" {
			fmt.Fprintln(diagnostics, warning)
		}
	}
	settings.DashboardURL = strings.TrimSpace(settings.DashboardURL)
	if !initialURLValid {
		settings.DashboardURL = ""
	}
	return menubar.Run(ctx, settings, !initialURLValid, save, versionChanged)
}
