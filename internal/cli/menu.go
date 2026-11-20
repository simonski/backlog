package cli

import (
	"context"
	"fmt"
	"io"

	"harness.engineering/internal/local"
	"harness.engineering/internal/menubar"
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
	if err := local.ValidateAddress(address); err != nil {
		return err
	}

	base := "http://" + address
	client := serverHTTPClient()
	if !checkServerVersion(ctx, client, base, diagnostics) && ctx.Err() == nil {
		fmt.Fprintln(diagnostics, "Backlog server version unavailable. The menu remains available.")
	}
	client.CloseIdleConnections()
	return menubar.Run(ctx, base+"/")
}
