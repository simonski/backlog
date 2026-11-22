package cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const launchAgentLabel = "local.backlog"

func (runtime serverRuntime) setupLaunchAgent(ctx context.Context, address, database string, out io.Writer) error {
	lock, err := runtime.lock(false)
	if err != nil {
		return err
	}
	lock.Close()
	host, port, splitErr := net.SplitHostPort(address)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if splitErr != nil || portErr != nil || number == 0 {
		return fmt.Errorf("background service requires a fixed host:port address, got %q", address)
	}
	if report := probeBindAddress(ctx, address); report.Port == "listening" {
		return bindConflict(address, report, syscall.EADDRINUSE)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	executable, err := executablePath()
	if err != nil {
		return err
	}
	args := []string{executable, "serve", "-addr", address}
	if database != "" {
		absolute, err := filepath.Abs(database)
		if err != nil {
			return err
		}
		args = append(args, "-db", absolute)
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	log := filepath.Join(home, "Library", "Logs", "backlog.log")
	alreadyLoaded, err := startLaunchAgent(domain, plist, log, address, args, func(args ...string) error {
		output, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("launchctl %s: %w (%s)", args[0], err, strings.TrimSpace(string(output)))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if alreadyLoaded {
		return errServerRunning
	}
	target := domain + "/" + launchAgentLabel
	if err := runtime.waitForLaunchAgent(ctx, target); err != nil {
		output, stopErr := runLaunchctl(context.WithoutCancel(ctx), "bootout", target)
		if stopErr != nil {
			err = errors.Join(err, fmt.Errorf("unload %s: %w (%s)", target, stopErr, strings.TrimSpace(string(output))))
		}
		return fmt.Errorf("start the background service (see %s): %w", log, err)
	}
	fmt.Fprintln(out, "Registered the background service. Backlog is ready.")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	fmt.Fprintf(out, "Dashboard: http://%s\nError log: %s\nAccess log: %s\n", net.JoinHostPort(host, port), log, accessLogPath(log))
	fmt.Fprintf(out, "Status: launchctl print %s/%s\nStop until next login: launchctl bootout %s/%s\n", domain, launchAgentLabel, domain, launchAgentLabel)
	return nil
}

func (runtime serverRuntime) waitForLaunchAgent(ctx context.Context, target string) error {
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if instance, err := runtime.control(wait, http.MethodGet, "/status"); err == nil {
			if job, err := runLaunchctl(wait, "print", target); err == nil && launchdField(string(job), "pid") == strconv.Itoa(instance.PID) {
				return nil
			}
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}

func startLaunchAgent(domain, plist, log, address string, args []string, launchctl func(...string) error) (bool, error) {
	service := domain + "/" + launchAgentLabel
	contents := launchAgentPlist(args, log, accessLogPath(log))
	legacy := launchAgentPlist(args, log, "")
	existing, readErr := os.ReadFile(plist)
	upgrade := readErr == nil && bytes.Equal(existing, legacy)
	if upgrade {
		info, err := os.Lstat(plist)
		upgrade = err == nil && info.Mode().IsRegular()
	}
	loaded := launchctl("print", service) == nil
	if loaded {
		if readErr == nil && bytes.Equal(existing, contents) {
			return true, nil
		}
		if !upgrade {
			return false, fmt.Errorf("service %s is already registered with a different configuration; inspect it with launchctl print %s", service, service)
		}
	} else {
		listener, err := listenServer(address)
		if errors.Is(err, syscall.EADDRINUSE) {
			return false, fmt.Errorf("%s is already in use; stop the foreground backlog serve or choose another port with -p", address)
		}
		if err != nil {
			return false, fmt.Errorf("check listen address %s: %w", address, err)
		}
		if err := listener.Close(); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		return false, err
	}
	if upgrade {
		if loaded {
			if err := launchctl("bootout", service); err != nil {
				return false, err
			}
		}
		if err := upgradeLaunchAgentFile(plist, legacy, contents); err != nil {
			if loaded {
				err = errors.Join(err, launchctl("bootstrap", domain, plist))
			}
			return false, err
		}
	} else if _, err := installFile(plist, contents); errors.Is(err, os.ErrExist) {
		return false, fmt.Errorf("a different service configuration already exists at %s; review it before running setup again", plist)
	} else if err != nil {
		return false, err
	}
	if err := launchctl("bootstrap", domain, plist); err != nil {
		// Another setup process may have registered the same file first.
		if launchctl("print", service) == nil {
			return true, nil
		}
		return false, fmt.Errorf("%w; the configuration is saved at %s, and setup can be run again from a macOS login session", err, plist)
	}
	return false, nil
}

func upgradeLaunchAgentFile(path string, old, updated []byte) error {
	staged, err := stageFile(filepath.Dir(path), updated)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !bytes.Equal(current, old) {
		return fmt.Errorf("launch agent configuration changed at %s", path)
	}
	return os.Rename(staged, path)
}

func accessLogPath(errorLog string) string {
	return filepath.Join(filepath.Dir(errorLog), "backlog-access.log")
}

func launchAgentPlist(args []string, log, accessLog string) []byte {
	var plist bytes.Buffer
	fmt.Fprintln(&plist, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>`+launchAgentLabel+`</string>
  <key>ProgramArguments</key>
  <array>`)
	for _, arg := range args {
		plist.WriteString("    <string>")
		xml.EscapeText(&plist, []byte(arg))
		plist.WriteString("</string>\n")
	}
	plist.WriteString("  </array>\n  <key>KeepAlive</key>\n  <true/>\n  <key>StandardErrorPath</key>\n  <string>")
	xml.EscapeText(&plist, []byte(log))
	plist.WriteString("</string>\n")
	if accessLog != "" {
		plist.WriteString("  <key>StandardOutPath</key>\n  <string>")
		xml.EscapeText(&plist, []byte(accessLog))
		plist.WriteString("</string>\n")
	}
	plist.WriteString("</dict>\n</plist>\n")
	return plist.Bytes()
}
