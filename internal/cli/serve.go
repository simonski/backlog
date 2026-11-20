package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/server"
	"harness.engineering/internal/storage"
	"harness.engineering/internal/version"
)

func serverFlags(flags *flag.FlagSet, address, database *string) func() error {
	flags.StringVar(database, "db", "", "SQLite path (defaults to harness-engineering/telemetry.db in the user config directory)")
	return addressFlags(flags, address, local.DefaultListenAddress, "bind address and port")
}

func serve(ctx context.Context, args []string, diagnostics io.Writer) error {
	flags := newFlags("serve", diagnostics, " [options]")
	var address, path string
	resolveAddress := serverFlags(flags, &address, &path)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if path == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("locate user configuration directory: %w", err)
		}
		path = filepath.Join(config, "harness-engineering", "telemetry.db")
	}

	executable, err := executablePath()
	var known executableRevision
	buildID := version.Server
	if err == nil {
		known, err = readExecutable(executable)
	}
	if err == nil {
		buildID = known.digest
	} else {
		fmt.Fprintf(diagnostics, "Automatic restart unavailable: %v\n", err)
	}

	for {
		replacement, port, err := runServer(ctx, address, path, executable, known, buildID, diagnostics)
		if err != nil || replacement.info == nil || ctx.Err() != nil {
			return err
		}
		fmt.Fprintln(diagnostics, "Backlog executable updated. Restarting server.")
		relaunchArgs := os.Args
		host, requestedPort, _ := net.SplitHostPort(address)
		if requestedPort == "0" {
			portText := strconv.Itoa(port)
			address = net.JoinHostPort(host, portText)
			relaunchArgs = append(append([]string(nil), os.Args...), "-p", portText)
		}
		if err := syscall.Exec(executable, relaunchArgs, os.Environ()); err != nil {
			fmt.Fprintf(diagnostics, "Upgrade restart failed: %v. Resuming the current build.\n", err)
		}
		known = replacement
	}
}

func listenServer(address string) (net.Listener, error) {
	network := "tcp"
	if strings.HasPrefix(address, "0.0.0.0:") {
		network = "tcp4"
	}
	return net.Listen(network, address)
}

func runServer(ctx context.Context, address, path, executable string, known executableRevision, buildID string, diagnostics io.Writer) (executableRevision, int, error) {
	var replacement executableRevision
	listener, err := listenServer(address)
	if err != nil {
		return replacement, 0, err
	}
	defer listener.Close()
	store, err := storage.Open(ctx, path)
	if err != nil {
		return replacement, 0, fmt.Errorf("open SQLite: %w", err)
	}
	defer store.Close()

	httpServer := &http.Server{
		Handler:           server.Handler(store, buildID),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       45 * time.Second,
	}
	done := make(chan error, 1)
	bound := listener.Addr().(*net.TCPAddr)
	if bound.IP.IsUnspecified() {
		host := "localhost"
		if bound.IP.To4() != nil {
			host = "127.0.0.1"
		}
		fmt.Fprintf(diagnostics, "Local dashboard: http://%s\n", net.JoinHostPort(host, strconv.Itoa(bound.Port)))
	} else {
		fmt.Fprintf(diagnostics, "Local dashboard: http://%s\n", bound)
	}
	fmt.Fprintf(diagnostics, "Listening on %s\n", bound)
	go func() { done <- httpServer.Serve(listener) }()

	watch, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	select {
	case err := <-done:
		return replacement, bound.Port, err
	case <-ctx.Done():
	case replacement = <-watchExecutable(watch, executable, known):
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdown); err != nil {
		httpServer.Close()
		if replacement.info == nil {
			return replacement, bound.Port, err
		}
		fmt.Fprintf(diagnostics, "Server drain before restart: %v\n", err)
	}
	return replacement, bound.Port, nil
}
