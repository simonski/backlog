package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/version"
)

type statusReport struct {
	BacklogURL      string `json:"backlog_url"`
	LocalURL        string `json:"local_url"`
	Port            string `json:"port"`
	ClientVersion   string `json:"client_version"`
	BundledVersion  string `json:"bundled_server_version"`
	Server          string `json:"server"`
	ServerVersion   string `json:"server_version,omitempty"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
	ServeProcess    string `json:"serve_process"`
	ServePIDs       []int  `json:"serve_pids"`
	Background      string `json:"background_service"`
	Advice          string `json:"advice,omitempty"`
}

func showStatus(ctx context.Context, command string, args []string, out, diagnostics io.Writer) error {
	flags := newFlags(command, diagnostics, " [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "local server host:port")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if err := local.ValidateAddress(address); err != nil {
		return err
	}

	report := statusReport{
		LocalURL: "http://" + address, ServePIDs: []int{},
		ClientVersion: version.Client, BundledVersion: version.Server,
	}
	report.BacklogURL = strings.TrimSpace(os.Getenv("BACKLOG_URL"))
	if report.BacklogURL == "" {
		report.BacklogURL = report.LocalURL
	}

	processCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	pids, err := servePIDs(processCtx)
	cancel()
	switch {
	case err != nil:
		report.ServeProcess = "check unavailable"
	case len(pids) == 0:
		report.ServeProcess = "not detected"
	default:
		report.ServeProcess, report.ServePIDs = "running", pids
	}
	backgroundCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	report.Background = backgroundServiceStatus(backgroundCtx)
	cancel()
	probeServer(ctx, address, &report, diagnostics)
	report.Advice = statusAdvice(report)

	if *asJSON {
		err = writeJSON(out, report)
	} else {
		err = writeStatus(out, address, report)
	}
	if err != nil {
		return err
	}
	if report.ServerVersion == "" {
		return fmt.Errorf("server version unavailable at %s", report.LocalURL)
	}
	return nil
}

func statusAdvice(report statusReport) string {
	running := report.ServerVersion
	if running == "" {
		if report.Port == "not listening" {
			return "Run backlog restart to see how to start the launchd service on macOS."
		}
		return ""
	}
	if version.Warning(report.BundledVersion, running) != "" && version.Older(running, report.BundledVersion) {
		return "Install the new build at the launchd job's executable path, then run backlog restart for the restart command."
	}
	if version.Warning(report.ClientVersion, running) == "" {
		return ""
	}
	if version.Older(report.ClientVersion, running) {
		return "Upgrade the Backlog CLI to the running server's release line."
	}
	if version.Warning(report.BundledVersion, running) == "" {
		return fmt.Sprintf("The running server is on this binary's bundled server release line (%s).", report.BundledVersion)
	}
	return "Check that the CLI and launchd job use the intended Backlog installation."
}

func probeServer(ctx context.Context, address string, report *statusReport, diagnostics io.Writer) {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		report.Port = "check unavailable"
		if errors.Is(err, syscall.ECONNREFUSED) {
			report.Port = "not listening"
		}
		report.Server = "unreachable"
		return
	}
	conn.Close()
	report.Port = "listening"
	client := serverHTTPClient()
	client.Timeout = 2 * time.Second
	defer client.CloseIdleConnections()
	advertised, err := readServerVersion(ctx, client, report.LocalURL)
	if err != nil || advertised.Protocol < 1 {
		report.Server = "listener did not identify as Backlog"
		return
	}
	report.Server = "running"
	report.ServerVersion = advertised.Version
	report.ProtocolVersion = advertised.Protocol
	if warning := version.Warning(version.Client, advertised.Version); warning != "" {
		fmt.Fprintln(diagnostics, warning)
	}
}

func writeStatus(out io.Writer, address string, report statusReport) error {
	process := report.ServeProcess
	if len(report.ServePIDs) > 0 {
		pids := make([]string, 0, len(report.ServePIDs))
		for _, pid := range report.ServePIDs {
			pids = append(pids, strconv.Itoa(pid))
		}
		process += " (PID " + strings.Join(pids, ", ") + ")"
	}
	server := report.Server
	if report.ServerVersion != "" {
		server += fmt.Sprintf(" (version %s, protocol %d)", report.ServerVersion, report.ProtocolVersion)
	}
	_, err := fmt.Fprintf(out, "BACKLOG_URL:     %s\nLocal endpoint:  %s\nPort %s: %s\nCLI version:     %s (bundled server %s)\nBacklog server:  %s\nbacklog serve:   %s\nBackground:      %s\n",
		report.BacklogURL, report.LocalURL, address, report.Port, report.ClientVersion, report.BundledVersion, server, process, report.Background)
	if err == nil && report.Advice != "" {
		_, err = fmt.Fprintf(out, "Advice:          %s\n", report.Advice)
	}
	return err
}
