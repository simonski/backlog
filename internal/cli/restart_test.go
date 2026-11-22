package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestartCommand(t *testing.T) {
	const domain = "gui/501"
	target := domain + "/" + launchAgentLabel + ".session"
	command, err := restartCommand([]string{target}, domain)
	if err != nil || command != "launchctl kickstart -k "+target {
		t.Fatalf("registered job: %q (%v)", command, err)
	}
	if _, err := restartCommand([]string{target, domain + "/" + launchAgentLabel}, domain); err == nil {
		t.Fatal("expected an error when two jobs are registered")
	}
	home := filepath.Join(t.TempDir(), "Simon's Home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if command, err := restartCommand(nil, domain); err != nil || !strings.HasSuffix(command, " setup") {
		t.Fatalf("missing plist: %q (%v)", command, err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	command, err = restartCommand(nil, domain)
	want := "launchctl bootstrap " + domain + " '" + strings.ReplaceAll(path, "'", "'\\''") + "'"
	if err != nil || command != want {
		t.Fatalf("installed plist: %q (%v), want %q", command, err, want)
	}
}
