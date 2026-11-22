package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func TestUpgradeLeavesCurrentAndNewerServersAlone(t *testing.T) {
	for _, scenario := range []struct {
		name, advertised, output, diagnostic string
		protocol, exit                       int
	}{
		{"current", version.Server, "already on " + version.Server, "", version.Protocol, 0},
		{"newer", "99.0.0", "", "upgrade the client", version.Protocol, 1},
		{"different protocol", version.Server, "", "bundled server", version.Protocol + 1, 1},
		{"unavailable", "invalid", "", "server version unavailable", 0, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/version" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				json.NewEncoder(w).Encode(wire.ServerVersion{Version: scenario.advertised, Protocol: scenario.protocol})
			}))
			defer service.Close()
			var output, diagnostics bytes.Buffer
			exit := (App{}).Run(t.Context(), []string{"upgrade", "-addr", strings.TrimPrefix(service.URL, "http://")}, nil, &output, &diagnostics)
			if exit != scenario.exit || !strings.Contains(output.String(), scenario.output) ||
				!strings.Contains(diagnostics.String(), scenario.diagnostic) ||
				(scenario.diagnostic == "" && diagnostics.Len() != 0) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", exit, &output, &diagnostics)
			}
		})
	}
}

func TestUpgradeRestoresLaunchAgentAfterFailedVerification(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("plutil is available on macOS")
	}
	const target = "gui/501/local.backlog"
	directory := t.TempDir()
	plist := filepath.Join(directory, launchAgentLabel+".plist")
	db := filepath.Join(directory, "backlog & data.db")
	oldPath, newPath := "/old/backlog", "/new/backlog"
	original := launchAgentPlist([]string{oldPath, "serve", "-addr", "127.0.0.1:15009", "-db", db},
		filepath.Join(directory, "stderr.log"), filepath.Join(directory, "stdout.log"))
	if err := os.WriteFile(plist, original, 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := changeLaunchdProgram(t.Context(), plist, original, launchAgentLabel, oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}

	unhealthy := errors.New("replacement did not become ready")
	var commands []string
	run := func(_ context.Context, args ...string) error {
		commands = append(commands, args[0])
		return nil
	}
	err = replaceLaunchdProgram(t.Context(), target, plist, original, updated, run, func(ctx context.Context) error {
		args, err := exec.CommandContext(ctx, "/usr/bin/plutil", "-extract", "ProgramArguments", "json", "-o", "-", plist).Output()
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := json.Unmarshal(args, &got); err != nil || !slices.Equal(got, []string{newPath, "serve", "-addr", "127.0.0.1:15009", "-db", db}) {
			t.Fatalf("replacement arguments %q (%v)", args, err)
		}
		return unhealthy
	})
	if !errors.Is(err, unhealthy) || !slices.Equal(commands, []string{"bootout", "bootstrap", "print", "bootout", "bootstrap"}) {
		t.Fatalf("rollback commands %v (%v)", commands, err)
	}
	if restored, err := os.ReadFile(plist); err != nil || !bytes.Equal(restored, original) {
		t.Fatalf("original plist was not restored (%v)", err)
	}
}
