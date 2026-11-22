package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func upgradeServer(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("upgrade", diagnostics, " [options]")
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

	base := "http://" + address
	client := serverHTTPClient()
	client.Timeout = time.Second
	defer client.CloseIdleConnections()
	probe := func(ctx context.Context) (wire.ServerVersion, error) {
		return readServerVersion(ctx, client, base)
	}
	previous, err := probe(ctx)
	if err != nil || previous.NetworkProtocol() == "" {
		return fmt.Errorf("cannot upgrade Backlog at %s (server version unavailable)", base)
	}
	if previous.Version == version.Server && previous.NetworkProtocol() == version.Protocol {
		_, err := fmt.Fprintf(out, "Backlog server at %s is already on %s.\n", base, version.Server)
		return err
	}
	if err := checkCompatibility(previous, diagnostics); err != nil {
		fmt.Fprintln(diagnostics, err)
	}
	if !version.Older(previous.Version, version.Server) {
		return fmt.Errorf("server %s uses protocol %s, bundled server %s uses protocol %s", previous.Version, previous.NetworkProtocol(), version.Server, version.Protocol)
	}
	if runtime.GOOS != "darwin" {
		return errors.New("automatic upgrade requires the macOS Backlog launchd service. Update the executable used by your server supervisor and restart it")
	}
	lookup, cancel := context.WithTimeout(ctx, 3*time.Second)
	managed := managedListeners(lookup, address)
	cancel()
	if len(managed) != 1 {
		// Replacing the executable may already have triggered the server's watcher.
		if current, err := probe(ctx); err == nil && current.Version == version.Server && current.NetworkProtocol() == version.Protocol {
			_, err := fmt.Fprintf(out, "Backlog server at %s is already on %s.\n", base, version.Server)
			return err
		}
		return fmt.Errorf("Backlog %s at %s is not owned by a known launchd job. Update the executable used by your server supervisor and restart it", previous.Version, base)
	}
	var target string
	for _, job := range managed {
		target = job
	}
	executable, err := executablePath()
	if err != nil {
		return err
	}
	if err := upgradeLaunchdServer(ctx, target, executable, func(ctx context.Context) error {
		return waitForServerUpgrade(ctx, probe)
	}); err != nil {
		return fmt.Errorf("upgrade Backlog at %s (%w)", base, err)
	}
	_, err = fmt.Fprintf(out, "Upgraded Backlog server at %s from %s to %s.\n", base, previous.Version, version.Server)
	return err
}

func waitForServerUpgrade(ctx context.Context, probe func(context.Context) (wire.ServerVersion, error)) error {
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	last := "server version unavailable"
	for {
		if current, err := probe(wait); err == nil && current.NetworkProtocol() != "" {
			if current.Version == version.Server && current.NetworkProtocol() == version.Protocol {
				return nil
			}
			last = fmt.Sprintf("server reports version %s and protocol %s", current.Version, current.NetworkProtocol())
		}
		select {
		case <-wait.Done():
			return fmt.Errorf("waiting for server %s (last check: %s): %w", version.Server, last, wait.Err())
		case <-ticker.C:
		}
	}
}

func upgradeLaunchdServer(ctx context.Context, target, executable string, verify func(context.Context) error) error {
	run := func(ctx context.Context, args ...string) error {
		output, err := runLaunchctl(ctx, args...)
		if err != nil {
			return fmt.Errorf("launchctl %s: %w (%s)", args[0], err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	job, err := runLaunchctl(ctx, "print", target)
	if err != nil {
		return fmt.Errorf("inspect launchd job %s: %w", target, err)
	}
	program := launchdField(string(job), "program")
	if !filepath.IsAbs(program) {
		return fmt.Errorf("cannot identify executable for launchd job %s", target)
	}
	selected, err := os.Stat(executable)
	if err != nil {
		return err
	}
	if current, err := os.Stat(program); err == nil && os.SameFile(current, selected) {
		if err := run(ctx, "kickstart", "-k", target); err != nil {
			return err
		}
		return verify(ctx)
	}

	plist := launchdField(string(job), "path")
	if !filepath.IsAbs(plist) || filepath.Base(plist) != filepath.Base(target)+".plist" {
		return fmt.Errorf("launchd job %s does not have an identifiable Backlog plist", target)
	}
	if info, err := os.Lstat(plist); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("launch agent configuration is not a regular file at %s", plist)
	}
	original, err := os.ReadFile(plist)
	if err != nil {
		return err
	}
	updated, err := changeLaunchdProgram(ctx, plist, original, filepath.Base(target), program, executable)
	if err != nil {
		return err
	}
	return replaceLaunchdProgram(ctx, target, plist, original, updated, run, verify)
}

func runLaunchctl(ctx context.Context, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return exec.CommandContext(commandCtx, "/bin/launchctl", args...).CombinedOutput()
}

func launchdField(job, field string) string {
	for _, line := range strings.Split(job, "\n") {
		if value, ok := strings.CutPrefix(line, "\t"+field+" = "); ok {
			return value
		}
	}
	return ""
}

func changeLaunchdProgram(ctx context.Context, plist string, original []byte, label, program, executable string) ([]byte, error) {
	staged, err := stageFile(filepath.Dir(plist), original)
	if err != nil {
		return nil, err
	}
	defer os.Remove(staged)
	plutil := func(args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "/usr/bin/plutil", append(args, staged)...).Output()
	}
	if output, err := plutil("-extract", "Label", "raw", "-expect", "string", "-n"); err != nil || string(output) != label {
		return nil, fmt.Errorf("unexpected Label in Backlog launch agent %s", plist)
	}
	output, err := plutil("-extract", "ProgramArguments", "json", "-o", "-")
	var arguments []string
	if err != nil || json.Unmarshal(output, &arguments) != nil || len(arguments) < 2 || arguments[0] != program || arguments[1] != "serve" {
		return nil, fmt.Errorf("unexpected ProgramArguments in Backlog launch agent %s", plist)
	}
	arguments[0] = executable
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	if _, err := plutil("-replace", "ProgramArguments", "-json", string(encoded)); err != nil {
		return nil, fmt.Errorf("set launch agent executable: %w", err)
	}
	if _, err := plutil("-type", "Program"); err == nil {
		if _, err := plutil("-replace", "Program", "-string", executable); err != nil {
			return nil, fmt.Errorf("set launch agent program: %w", err)
		}
	}
	return os.ReadFile(staged)
}

func replaceLaunchdProgram(ctx context.Context, target, plist string, original, updated []byte,
	run func(context.Context, ...string) error, verify func(context.Context) error) error {
	domain := filepath.Dir(target)
	if err := unloadLaunchdJob(ctx, target, run); err != nil {
		return err
	}
	if err := upgradeLaunchAgentFile(plist, original, updated); err != nil {
		return errors.Join(err, run(context.WithoutCancel(ctx), "bootstrap", domain, plist))
	}
	err := run(ctx, "bootstrap", domain, plist)
	if err == nil {
		if err = verify(ctx); err == nil {
			return nil
		}
	}
	recover := context.WithoutCancel(ctx)
	if run(recover, "print", target) == nil {
		if stopErr := unloadLaunchdJob(recover, target, run); stopErr != nil {
			return errors.Join(err, fmt.Errorf("could not stop replacement launch agent: %w", stopErr))
		}
	}
	if rollbackErr := upgradeLaunchAgentFile(plist, updated, original); rollbackErr != nil {
		return errors.Join(err, fmt.Errorf("could not restore original launch agent: %w", rollbackErr))
	}
	return errors.Join(err, run(recover, "bootstrap", domain, plist))
}

func unloadLaunchdJob(ctx context.Context, target string, run func(context.Context, ...string) error) error {
	if err := run(ctx, "bootout", target); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := run(wait, "print", target)
		if wait.Err() == nil && err != nil {
			return nil
		}
		select {
		case <-wait.Done():
			return fmt.Errorf("wait for launchd to unload %s: %w", target, wait.Err())
		case <-ticker.C:
		}
	}
}
