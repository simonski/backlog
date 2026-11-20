package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSetupInstallsSkillOnlyAfterConfirmation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "report-harness-progress", "SKILL.md")
	app := App{ReportingSkill: []byte("# Reporting skill\n")}
	args := []string{"setup", "-dir", directory}

	declineService := ""
	if runtime.GOOS == "darwin" {
		declineService = "no\n"
	}
	for _, tc := range []struct {
		answers string
		code    int
		want    string
	}{
		{declineService, 1, ""},
		{declineService + "no\n", 0, ""},
		{declineService + "maybe\n\n", 0, "Installed reporting skill"},
		{declineService + "yes\n", 0, "Reporting skill already installed"},
	} {
		var out, diagnostics bytes.Buffer
		code := app.Run(t.Context(), args, strings.NewReader(tc.answers), &out, &diagnostics)
		if code != tc.code || !strings.Contains(out.String(), tc.want) {
			t.Fatalf("answers %q: exit %d, stdout %q, stderr %q", tc.answers, code, &out, &diagnostics)
		}
		got, err := os.ReadFile(path)
		if tc.want == "" {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("skill should be absent, read returned %v", err)
			}
		} else if err != nil || !bytes.Equal(got, app.ReportingSkill) {
			t.Fatalf("skill contents %q: %v", got, err)
		}
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
		!reflect.DeepEqual(config.LabelAndLog, []string{"local.backlog", log}) || config.KeepAlive == nil {
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
