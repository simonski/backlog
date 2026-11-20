package cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const launchAgentLabel = "local.backlog"

func setupLaunchAgent(ctx context.Context, address, database string, out io.Writer) error {
	host, port, splitErr := net.SplitHostPort(address)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if splitErr != nil || portErr != nil || number == 0 {
		return fmt.Errorf("background service requires a fixed host:port address, got %q", address)
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
		fmt.Fprintln(out, "Background service is already registered.")
	} else {
		fmt.Fprintln(out, "Registered the background service and requested startup.")
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	fmt.Fprintf(out, "Dashboard: http://%s\nLogs: %s\n", net.JoinHostPort(host, port), log)
	fmt.Fprintf(out, "Status: launchctl print %s/%s\nStop until next login: launchctl bootout %s/%s\n", domain, launchAgentLabel, domain, launchAgentLabel)
	return nil
}

func startLaunchAgent(domain, plist, log, address string, args []string, launchctl func(...string) error) (bool, error) {
	service := domain + "/" + launchAgentLabel
	contents := launchAgentPlist(args, log)
	if launchctl("print", service) == nil {
		if existing, err := os.ReadFile(plist); err != nil || !bytes.Equal(existing, contents) {
			return false, fmt.Errorf("service %s is already registered with a different configuration; inspect it with launchctl print %s", service, service)
		}
		return true, nil
	}

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
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		return false, err
	}
	if _, err := installFile(plist, contents); errors.Is(err, os.ErrExist) {
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

func launchAgentPlist(args []string, log string) []byte {
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
	plist.WriteString("</string>\n</dict>\n</plist>\n")
	return plist.Bytes()
}
