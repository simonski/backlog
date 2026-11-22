package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func TestStatusDistinguishesBacklogForeignListenerAndClosedPort(t *testing.T) {
	for _, tc := range []struct {
		name, advertised, portStatus, serverStatus, warning string
		protocol, code                                      int
	}{
		{"backlog", version.Client, "listening", "running", "", 1, 0},
		{"older backlog", "0.1.0", "listening", "running", "upgrade the server", 1, 0},
		{"patch mismatch", version.Client[:strings.LastIndex(version.Client, ".")] + ".9", "listening", "running", "", 1, 0},
		{"foreign HTTP listener", "invalid", "listening", "listener did not identify as Backlog", "server version unavailable", 0, 1},
		{"version without protocol", version.Client, "listening", "listener did not identify as Backlog", "server version unavailable", 0, 1},
		{"closed port", "", "not listening", "unreachable", "server version unavailable", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const dashboard = "https://mini.example.ts.net:15001/"
			t.Setenv("BACKLOG_URL", dashboard)
			var address string
			if tc.advertised == "" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				address = listener.Addr().String()
				listener.Close()
			} else {
				service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api/version" || r.Method != http.MethodGet {
						http.NotFound(w, r)
						return
					}
					json.NewEncoder(w).Encode(wire.ServerVersion{Version: tc.advertised, Protocol: tc.protocol})
				}))
				defer service.Close()
				address = strings.TrimPrefix(service.URL, "http://")
			}
			var out, diagnostics bytes.Buffer
			code := (App{}).Run(t.Context(), []string{"status", "-addr", address, "-json"}, nil, &out, &diagnostics)
			var report statusReport
			if err := json.Unmarshal(out.Bytes(), &report); err != nil {
				t.Fatalf("invalid status output %q: %v", &out, err)
			}
			if code != tc.code || report.Port != tc.portStatus || report.Server != tc.serverStatus ||
				report.BacklogURL != dashboard || report.LocalURL != "http://"+address {
				t.Fatalf("exit=%d status=%+v stderr=%q", code, report, &diagnostics)
			}
			if tc.warning == "" && diagnostics.Len() > 0 || !strings.Contains(diagnostics.String(), tc.warning) {
				t.Fatalf("stderr=%q, expected %q", &diagnostics, tc.warning)
			}
			if tc.code == 0 && (report.ServerVersion != tc.advertised || report.ProtocolVersion != tc.protocol) ||
				tc.code != 0 && report.ServerVersion != "" {
				t.Fatalf("incorrect server version: %+v", report)
			}
		})
	}
}

func TestStatusTextAndPortOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"server_version":%q,"protocol_version":1}`, version.Client)
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKLOG_URL", "")
	t.Setenv("HARNESS_ADDR", "127.0.0.1:1")
	var out, diagnostics bytes.Buffer
	code := (App{}).Run(t.Context(), []string{"ps", "-p", port}, nil, &out, &diagnostics)
	if code != 0 || diagnostics.Len() != 0 || !strings.Contains(out.String(), "BACKLOG_URL:     "+server.URL) ||
		!strings.Contains(out.String(), "Backlog server:  running") || !strings.Contains(out.String(), "backlog serve:") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
	}
}

func TestStatusVersionAdvice(t *testing.T) {
	for _, tc := range []struct {
		running, want string
	}{
		{"0.9.0", "backlog upgrade"},
		{"0.11.0", "bundled server release line"},
		{"0.12.1", ""},
		{"0.13.0", "Upgrade the Backlog CLI"},
	} {
		report := statusReport{ClientVersion: "0.12.0", BundledVersion: "0.11.0", ServerVersion: tc.running}
		if got := statusAdvice(report); tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
			t.Errorf("server %s: advice %q, want %q", tc.running, got, tc.want)
		}
	}
}

func TestIsServeProcess(t *testing.T) {
	for _, tc := range []struct {
		executable, args string
		want             bool
	}{
		{"/path with spaces/backlog", "/path with spaces/backlog serve -p 15000", true},
		{"backlog", "/usr/local/bin/backlog serve", true},
		{"backlog", "backlog menu", false},
		{"backlog", "backlog status serve", false},
	} {
		if got := isServeProcess(tc.executable, tc.args); got != tc.want {
			t.Errorf("isServeProcess(%q, %q) = %t, want %t", tc.executable, tc.args, got, tc.want)
		}
	}
}
