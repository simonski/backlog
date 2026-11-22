package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"harness.engineering/internal/version"
)

func TestSetupClientPersistsOnlyTheDashboardURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("BACKLOG_URL", "")
	const dashboard = "https://backlog.example.ts.net/"
	var out, diagnostics bytes.Buffer
	answers := "client\nfile:///etc/hosts\n" + dashboard + "\n\n"
	code := (App{ReportingSkill: []byte("embedded skill")}).Run(t.Context(), []string{"setup"}, strings.NewReader(answers), &out, &diagnostics)
	if code != 0 || !strings.HasPrefix(out.String(), "Set up Backlog as a client or server?") ||
		!strings.Contains(out.String(), "Saved BACKLOG_URL="+dashboard) || diagnostics.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
	}
	if got, err := dashboardURL("fallback"); err != nil || got != dashboard {
		t.Fatalf("saved URL = %q (%v)", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("client setup touched the skills directory: %v", err)
	}
	t.Setenv("BACKLOG_URL", "https://override.example.ts.net/")
	if got, err := dashboardURL("fallback"); err != nil || got != "https://override.example.ts.net/" {
		t.Fatalf("environment override = %q (%v)", got, err)
	}
}

func TestSetupServerReviewsWithoutInstalling(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/version" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprintf(w, `{"server_version":%q,"protocol_version":1}`, version.Client)
	}))
	defer service.Close()
	var out, diagnostics bytes.Buffer
	args := []string{"setup", "-addr", strings.TrimPrefix(service.URL, "http://")}
	code := (App{}).Run(t.Context(), args, strings.NewReader("server\n"), &out, &diagnostics)
	if code != 0 || !strings.Contains(out.String(), "Backlog server:  running") ||
		!strings.Contains(out.String(), "Advice:") || diagnostics.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
	}
	path, err := dashboardURLPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("server review saved client settings: %v", err)
	}
}

func TestStartLaunchAgentRetriesAndReusesConfiguration(t *testing.T) {
	home := t.TempDir()
	plist := filepath.Join(home, "LaunchAgents", "local.backlog.plist")
	log := filepath.Join(home, "Logs", "backlog & server.log")
	args := []string{"/usr/local/bin/backlog", "serve", "-addr", "127.0.0.1:15000", "-db", "/path/Backlog <test>.db"}
	const domain = "gui/123"

	loaded := false
	bootstrapErr := errors.New("launchd unavailable")
	bootstraps := 0
	launchctl := func(got ...string) error {
		if reflect.DeepEqual(got, []string{"print", domain + "/local.backlog"}) {
			if loaded {
				return nil
			}
			return errors.New("service not loaded")
		}
		if !reflect.DeepEqual(got, []string{"bootstrap", domain, plist}) {
			t.Fatalf("unexpected launchctl arguments: %q", got)
		}
		bootstraps++
		loaded = bootstrapErr == nil
		return bootstrapErr
	}
	start := func(args []string) (bool, error) {
		return startLaunchAgent(domain, plist, log, "127.0.0.1:0", args, launchctl)
	}
	if _, err := start(args); !errors.Is(err, bootstrapErr) {
		t.Fatalf("expected bootstrap failure, got %v", err)
	}
	contents, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		LabelAndLog []string  `xml:"dict>string"`
		Arguments   []string  `xml:"dict>array>string"`
		KeepAlive   *struct{} `xml:"dict>true"`
	}
	if err := xml.Unmarshal(contents, &config); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Arguments, args) ||
		!reflect.DeepEqual(config.LabelAndLog, []string{"local.backlog", log, accessLogPath(log)}) || config.KeepAlive == nil {
		t.Fatalf("unexpected service configuration: %+v", config)
	}
	bootstrapErr = nil
	if already, err := start(args); err != nil || already {
		t.Fatalf("bootstrap retry returned already=%t, err=%v", already, err)
	}
	if already, err := start(args); err != nil || !already || bootstraps != 2 {
		t.Fatalf("repeat setup returned already=%t, err=%v after %d bootstraps", already, err, bootstraps)
	}
	if _, err := start([]string{args[0], "serve"}); err == nil || !strings.Contains(err.Error(), "different configuration") {
		t.Fatalf("expected conflict with the loaded configuration, got %v", err)
	}
	loaded = false
	if _, err := start([]string{args[0], "serve"}); err == nil || !strings.Contains(err.Error(), "different service configuration") {
		t.Fatalf("expected conflict with the saved configuration, got %v", err)
	}
	got, err := os.ReadFile(plist)
	if err != nil || !bytes.Equal(got, contents) {
		t.Fatalf("the existing service file changed: %s (%v)", got, err)
	}
}

func TestStartLaunchAgentAddsAccessLogToLegacyConfiguration(t *testing.T) {
	directory := t.TempDir()
	plist := filepath.Join(directory, "local.backlog.plist")
	log := filepath.Join(directory, "backlog.log")
	args := []string{"/usr/local/bin/backlog", "serve", "-addr", "127.0.0.1:15000"}
	if err := os.WriteFile(plist, launchAgentPlist(args, log, ""), 0o600); err != nil {
		t.Fatal(err)
	}
	const domain = "gui/123"
	want := [][]string{{"print", domain + "/local.backlog"}, {"bootout", domain + "/local.backlog"}, {"bootstrap", domain, plist}}
	var commands [][]string
	already, err := startLaunchAgent(domain, plist, log, "127.0.0.1:15000", args, func(command ...string) error {
		commands = append(commands, command)
		return nil
	})
	if err != nil || already || !reflect.DeepEqual(commands, want) {
		t.Fatalf("upgrade failed: already=%t launchctl=%v err=%v", already, commands, err)
	}
	contents, err := os.ReadFile(plist)
	if err != nil || !bytes.Equal(contents, launchAgentPlist(args, log, accessLogPath(log))) {
		t.Fatalf("unexpected updated plist %q: %v", contents, err)
	}
}

func TestStartLaunchAgentLeavesOccupiedPortAlone(t *testing.T) {
	listener, err := listenServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	home := t.TempDir()
	plist := filepath.Join(home, "local.backlog.plist")
	_, err = startLaunchAgent("gui/123", plist, filepath.Join(home, "backlog.log"), listener.Addr().String(), nil, func(args ...string) error {
		if args[0] != "print" {
			t.Fatalf("unexpected launchctl invocation: %q", args)
		}
		return errors.New("service not loaded")
	})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected occupied port error, got %v", err)
	}
	if _, err := os.Stat(plist); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected setup to leave no service file, stat returned %v", err)
	}
}
