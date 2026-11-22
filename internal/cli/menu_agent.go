package cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const menuAgentLabel = "local.backlog.menu"

func startMenuAgent(ctx context.Context, args []string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	target := domain + "/" + menuAgentLabel
	if job, err := runLaunchctl(ctx, "print", target); err == nil {
		if launchdField(string(job), "state") != "not running" || launchdField(string(job), "pid") != "" {
			// An existing worker is running or starting.
			return nil
		}
		if output, err := runLaunchctl(ctx, "bootout", target); err != nil {
			return fmt.Errorf("unregister stopped menu bar client: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	}

	executable, err := executablePath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	log := filepath.Join(home, "Library", "Logs", "backlog-menu.log")
	if err := os.MkdirAll(filepath.Dir(log), 0o755); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "backlog-menu-")
	if err != nil {
		return err
	}
	defer os.Remove(directory)
	plist := filepath.Join(directory, menuAgentLabel+".plist")
	defer os.Remove(plist)
	arguments := append([]string{executable, "menu", "-foreground"}, args...)
	if err := os.WriteFile(plist, menuAgentPlist(arguments, log), 0o600); err != nil {
		return err
	}

	output, err := runLaunchctl(ctx, "bootstrap", domain, plist)
	if err != nil {
		// A simultaneous invocation may have registered the job first.
		if _, lookupErr := runLaunchctl(ctx, "print", target); lookupErr != nil {
			return fmt.Errorf("start Backlog in a macOS GUI login session: %w (%s)", err, strings.TrimSpace(string(output)))
		}
	}
	if err := waitForMenuAgent(ctx, target); err != nil {
		return fmt.Errorf("Backlog menu bar client did not start. See %s (%w)", log, err)
	}
	return nil
}

func waitForMenuAgent(ctx context.Context, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, err := runLaunchctl(ctx, "print", target)
		if err == nil && launchdField(string(output), "pid") != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func menuAgentPlist(args []string, log string) []byte {
	var plist bytes.Buffer
	fmt.Fprintln(&plist, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>`+menuAgentLabel+`</string>
  <key>RunAtLoad</key>
  <true/>
  <key>ProgramArguments</key>
  <array>`)
	writeValue := func(value string) {
		plist.WriteString("    <string>")
		xml.EscapeText(&plist, []byte(value))
		plist.WriteString("</string>\n")
	}
	for _, arg := range args {
		writeValue(arg)
	}
	plist.WriteString("  </array>\n  <key>EnvironmentVariables</key>\n  <dict>\n")
	for _, key := range []string{"BACKLOG_URL", "BACKLOG_USERNAME", "HARNESS_ADDR"} {
		if value, present := os.LookupEnv(key); present {
			fmt.Fprintf(&plist, "    <key>%s</key>\n", key)
			writeValue(value)
		}
	}
	plist.WriteString("  </dict>\n  <key>StandardOutPath</key>\n")
	writeValue(log)
	plist.WriteString("  <key>StandardErrorPath</key>\n")
	writeValue(log)
	plist.WriteString("</dict>\n</plist>\n")
	return plist.Bytes()
}
