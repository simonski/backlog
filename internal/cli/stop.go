package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/wire"
)

func stopServer(ctx context.Context, address string, out, diagnostics io.Writer) error {
	address, err := loopbackBindAddress(address)
	if err != nil {
		return err
	}
	base := "http://" + address
	client := serverHTTPClient()
	defer client.CloseIdleConnections()
	advertised, err := readServerVersion(ctx, client, base)
	if err != nil {
		return fmt.Errorf("cannot stop Backlog at %s (server version unavailable)", base)
	}
	if advertised.NetworkProtocol() == "" {
		return fmt.Errorf("listener at %s did not identify as Backlog (server version unavailable)", base)
	}
	if err := checkCompatibility(advertised, diagnostics); err != nil {
		return err
	}
	launchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	managed := managedListeners(launchCtx, address)
	cancel()
	var receipt wire.StopResult
	if err := request(ctx, client, http.MethodPost, base+"/api/stop", []byte("{}"), &receipt, 64<<10); err != nil {
		return fmt.Errorf("stop request to Backlog %s at %s failed (%w)", advertised.Version, base, err)
	}
	if receipt.PID < 1 {
		return errors.New("Backlog did not return a valid shutdown receipt")
	}
	if target := managed[receipt.PID]; target != "" {
		launchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := exec.CommandContext(launchCtx, "/bin/launchctl", "bootout", target).Run(); err != nil &&
			(launchCtx.Err() != nil || exec.CommandContext(launchCtx, "/bin/launchctl", "print", target).Run() == nil) {
			return fmt.Errorf("Backlog accepted the stop request but launchd could not unload %s (%w)", target, err)
		}
	}
	if err := waitForStop(ctx, address); err != nil {
		return fmt.Errorf("Backlog accepted the stop request but %s is still reachable or could not be checked (%w)", base, err)
	}
	_, err = fmt.Fprintf(out, "Stopped Backlog at %s (PID %d).\n", base, receipt.PID)
	return err
}

func managedListeners(ctx context.Context, address string) map[int]string {
	managed := make(map[int]string)
	host, port, _ := net.SplitHostPort(address)
	for _, target := range launchAgentTargets() {
		output, err := exec.CommandContext(ctx, "/bin/launchctl", "print", target).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(output), "\n") {
			pid, found := strings.CutPrefix(strings.TrimSpace(line), "pid = ")
			if !found {
				continue
			}
			value, err := strconv.Atoi(pid)
			if err != nil || value < 1 {
				break
			}
			listeners, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-p", pid,
				"-iTCP:"+port, "-sTCP:LISTEN", "-Fn").Output()
			if err == nil {
				for _, entry := range strings.Split(string(listeners), "\n") {
					endpoint, ok := strings.CutPrefix(entry, "n")
					listenHost, _, err := net.SplitHostPort(endpoint)
					if ok && err == nil && (listenHost == "*" || listenHost == host ||
						(strings.EqualFold(host, "localhost") && local.IsLoopback(listenHost))) {
						managed[value] = target
						break
					}
				}
			}
			break
		}
	}
	return managed
}

func waitForStop(ctx context.Context, address string) error {
	wait, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	dialer := net.Dialer{Timeout: time.Second}
	for {
		conn, err := dialer.DialContext(wait, "tcp", address)
		if errors.Is(err, syscall.ECONNREFUSED) {
			return nil
		}
		if err == nil {
			conn.Close()
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}
