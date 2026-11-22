package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"harness.engineering/internal/local"
	"harness.engineering/internal/menubar"
	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func runMenu(ctx context.Context, args []string, diagnostics io.Writer) error {
	flags := newFlags("menu", diagnostics, " [options]")
	foreground := flags.Bool("foreground", false, "run in this process for debugging or supervision")
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
	if hasAddressOverride(flags) {
		if err := local.ValidateAddress(address); err != nil {
			return err
		}
	}
	if !*foreground && os.Getenv("XPC_SERVICE_NAME") != menuAgentLabel {
		return startMenuAgent(ctx, args)
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
	var lastVersion wire.ServerVersion
	checked, compatible := false, false
	checkVersion := func(advertised wire.ServerVersion) bool {
		if checked && lastVersion == advertised {
			return compatible
		}
		checked, lastVersion, compatible = true, advertised, false
		if !version.Valid(advertised.Version) {
			fmt.Fprintln(diagnostics, "Backlog server version unavailable.")
		} else if err := checkCompatibility(advertised, diagnostics); err != nil {
			fmt.Fprintln(diagnostics, err)
		} else {
			compatible = true
		}
		return compatible
	}
	settings.DashboardURL = strings.TrimSpace(settings.DashboardURL)
	if !initialURLValid {
		settings.DashboardURL = ""
	}
	return menubar.Run(ctx, settings, !initialURLValid, save, checkVersion)
}
