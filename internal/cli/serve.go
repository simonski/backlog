package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"harness.engineering/internal/local"
	"harness.engineering/internal/server"
	"harness.engineering/internal/storage"
	"harness.engineering/internal/version"
)

func serverFlags(flags *flag.FlagSet, address, database *string) func() error {
	flags.StringVar(database, "db", "", "SQLite path (defaults to harness-engineering/telemetry.db in the user config directory)")
	return addressFlags(flags, address, local.DefaultListenAddress, "bind address and port")
}

func (app App) serve(ctx context.Context, args []string, out, diagnostics io.Writer) (err error) {
	var ready *os.File
	if fd, parseErr := strconv.Atoi(os.Getenv(serverReadyFD)); parseErr == nil && fd >= 3 {
		ready = os.NewFile(uintptr(fd), "Backlog startup notification")
		defer func() {
			if ready != nil {
				if err != nil {
					_ = json.NewEncoder(ready).Encode(startupResult{Error: err.Error()})
				}
				ready.Close()
			}
		}()
	}
	flags := newFlags("serve", diagnostics, " [options]")
	var address, path string
	resolveAddress := serverFlags(flags, &address, &path)
	stop := flags.Bool("stop", false, "stop the local server (compatibility alias for backlog stop)")
	detach := flags.Bool("d", false, "start in the background")
	archiveHost := flags.String("tailscale-archive-host", "", "Tailscale Serve HTTPS hostname and optional port for archive actions")
	archiveUser := flags.String("tailscale-archive-user", "", "Tailscale login authorized to archive through Serve")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if *stop {
		if *detach {
			return errors.New("-d cannot be used with -stop")
		}
		if instance, err := app.runtime.control(ctx, http.MethodGet, "/status"); err == nil {
			selected := !hasAddressOverride(flags)
			if requested, requestErr := loopbackBindAddress(address); requestErr == nil {
				if running, addressErr := loopbackBindAddress(instance.Address); addressErr == nil && running == requested {
					selected = true
				}
			}
			if selected {
				return app.runtime.stopInstance(ctx, instance, out)
			}
		}
		return stopServer(ctx, address, out, diagnostics)
	}
	archiveAccess, err := server.NewTailscaleArchiveAccess(*archiveHost, *archiveUser, address)
	if err != nil {
		return err
	}
	path, err = databasePath(path)
	if err != nil {
		return err
	}
	if *detach {
		return startBackground(ctx, []string{"serve", "-addr", address, "-db", path,
			"-tailscale-archive-host", *archiveHost, "-tailscale-archive-user", *archiveUser}, out)
	}
	lock, err := app.runtime.lock(true)
	if err != nil {
		return err
	}
	defer lock.Close()

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
		replacement, port, err := app.runtime.runServer(ctx, address, path, executable, known, buildID, archiveAccess, out, diagnostics, func(address string) {
			if ready != nil {
				_ = json.NewEncoder(ready).Encode(startupResult{Address: address})
				ready.Close()
				ready = nil
			}
		})
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
		if err := relaunchServer(executable, relaunchArgs, lock); err != nil {
			fmt.Fprintf(diagnostics, "Upgrade restart failed: %v. Resuming the current build.\n", err)
		}
		known = replacement
	}
}

func relaunchServer(executable string, args []string, lock *os.File) error {
	fd := int(lock.Fd())
	if _, err := unix.FcntlInt(lock.Fd(), unix.F_SETFD, 0); err != nil {
		return err
	}
	defer unix.CloseOnExec(fd)
	env := slices.DeleteFunc(os.Environ(), func(value string) bool {
		return strings.HasPrefix(value, serverLockFD+"=") || strings.HasPrefix(value, serverReadyFD+"=")
	})
	return syscall.Exec(executable, args, append(env, serverLockFD+"="+strconv.Itoa(fd)))
}

func listenServer(address string) (net.Listener, error) {
	network := "tcp"
	if strings.HasPrefix(address, "0.0.0.0:") {
		network = "tcp4"
	}
	return net.Listen(network, address)
}

func loopbackBindAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	probeAddress := net.JoinHostPort(host, port)
	return probeAddress, local.ValidateAddress(probeAddress)
}

func probeBindAddress(ctx context.Context, address string) statusReport {
	probeAddress, err := loopbackBindAddress(address)
	if err != nil {
		return statusReport{}
	}
	report := statusReport{LocalURL: "http://" + probeAddress}
	probeServer(ctx, probeAddress, &report, io.Discard)
	return report
}

func bindConflict(address string, report statusReport, bindErr error) error {
	_, port, _ := net.SplitHostPort(address)
	if report.ServerVersion != "" {
		return fmt.Errorf("Backlog %s is already running at %s. Stop it before starting a server (%w)",
			report.ServerVersion, report.LocalURL, bindErr)
	}
	if report.Port == "listening" {
		return fmt.Errorf("port %s is already in use. The listener did not identify as Backlog. Stop it or choose a different port with -p (%w)",
			port, bindErr)
	}
	return fmt.Errorf("port %s is already in use. Stop the existing listener or choose a different port with -p (%w)", port, bindErr)
}

func (runtime serverRuntime) runServer(ctx context.Context, address, path, executable string, known executableRevision, buildID string, archiveAccess server.TailscaleArchiveAccess, out, diagnostics io.Writer, ready func(string)) (executableRevision, int, error) {
	var replacement executableRevision
	// A wildcard bind can succeed even with an existing loopback listener on macOS.
	if report := probeBindAddress(ctx, address); report.Port == "listening" {
		return replacement, 0, bindConflict(address, report, syscall.EADDRINUSE)
	}
	listener, err := listenServer(address)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			err = bindConflict(address, probeBindAddress(ctx, address), err)
		}
		return replacement, 0, err
	}
	defer listener.Close()
	store, err := storage.Open(ctx, path)
	if err != nil {
		return replacement, 0, fmt.Errorf("open SQLite: %w", err)
	}
	defer store.Close()

	stopCtx, requestStop := context.WithCancel(ctx)
	defer requestStop()
	handler := server.HandlerWithShutdown(store, buildID, archiveAccess, requestStop)
	httpServer := &http.Server{
		Handler:           server.AccessLog(out, handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       45 * time.Second,
	}
	done := make(chan error, 1)
	bound := listener.Addr().(*net.TCPAddr)
	closeControl, err := runtime.openControl(bound.String(), requestStop)
	if err != nil {
		return replacement, 0, fmt.Errorf("open Backlog control socket: %w", err)
	}
	defer closeControl()
	dashboard := bound.String()
	if bound.IP.IsUnspecified() {
		host := "localhost"
		if bound.IP.To4() != nil {
			host = "127.0.0.1"
		}
		dashboard = net.JoinHostPort(host, strconv.Itoa(bound.Port))
	}
	fmt.Fprintf(diagnostics, "Local dashboard: http://%s\n", dashboard)
	fmt.Fprintf(diagnostics, "Listening on %s\n", bound)
	go func() { done <- httpServer.Serve(listener) }()
	ready(dashboard)

	watch, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	select {
	case err := <-done:
		return replacement, bound.Port, err
	case <-stopCtx.Done():
	case replacement = <-watchExecutable(watch, executable, known):
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	drainErr := httpServer.Shutdown(shutdown)
	if stopCtx.Err() != nil {
		replacement = executableRevision{}
	}
	if drainErr != nil {
		httpServer.Close()
		if replacement.info == nil {
			return replacement, bound.Port, drainErr
		}
		fmt.Fprintf(diagnostics, "Server drain before restart: %v\n", drainErr)
	}
	return replacement, bound.Port, nil
}
