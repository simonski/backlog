package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func TestReportVersionCompatibility(t *testing.T) {
	parts := strings.Split(version.Client, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	cases := []struct {
		name          string
		serverVersion string
		resultCode    int
		diagnostic    string
	}{
		{"minor mismatch", fmt.Sprintf("%d.%d.0", major, minor+1), http.StatusOK, "upgrade the client"},
		{"major mismatch", fmt.Sprintf("%d.0.0", major+1), http.StatusOK, "upgrade the client"},
		{"older server", "0.0.9", http.StatusOK, "upgrade the server"},
		{"patch only", fmt.Sprintf("%d.%d.%d", major, minor, patch+1), http.StatusOK, ""},
		{"unknown version, supported report", "", http.StatusOK, ""},
		{"unknown version, unavailable report", "", http.StatusNotFound, "server version unavailable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			requests := make(chan string, 2)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				if tc.serverVersion == "" {
					http.NotFound(w, r)
					return
				}
				json.NewEncoder(w).Encode(wire.ServerVersion{Version: tc.serverVersion, Protocol: version.LegacyProtocol})
			})
			mux.HandleFunc("POST /api/reports", func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				w.WriteHeader(tc.resultCode)
				if tc.resultCode == http.StatusOK {
					json.NewEncoder(w).Encode(model.Result{ProjectID: 1, Reported: 1})
				}
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			var out, diagnostics bytes.Buffer
			address := strings.TrimPrefix(server.URL, "http://")
			code := (App{}).Run(t.Context(), []string{"task", "-addr", address, "-id", "task", "-title", "Work", "-json"}, nil, &out, &diagnostics)
			if len(requests) != 2 || <-requests != "/api/version" || <-requests != "/api/reports" {
				t.Fatal("expected a version request followed by the report")
			}
			switch {
			case tc.resultCode == http.StatusOK:
				var result model.Result
				if err := json.Unmarshal(out.Bytes(), &result); code != 0 || err != nil || result != (model.Result{ProjectID: 1, Reported: 1}) {
					t.Fatalf("invalid success receipt: exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
				}
				if want := "{\n  \"project_id\": 1,\n  \"reported\": 1\n}\n"; out.String() != want {
					t.Fatalf("expected indented receipt, got %q", &out)
				}
			default:
				if code == 0 || out.Len() != 0 {
					t.Fatalf("expected a failing exit and empty stdout: exit=%d stdout=%q", code, &out)
				}
			}

			if tc.diagnostic == "" && diagnostics.Len() != 0 {
				t.Fatalf("unexpected diagnostic: %q", &diagnostics)
			}
			if !strings.Contains(diagnostics.String(), tc.diagnostic) {
				t.Fatalf("missing diagnostic %q in %q", tc.diagnostic, &diagnostics)
			}
			if strings.HasPrefix(tc.diagnostic, "upgrade") &&
				(!strings.Contains(diagnostics.String(), version.Client) ||
					!strings.Contains(diagnostics.String(), tc.serverVersion)) {
				t.Fatalf("diagnostic omitted versions: %q", &diagnostics)
			}
			if tc.diagnostic == "upgrade the client" && !strings.Contains(diagnostics.String(), "brew upgrade simonski/backlog/backlog") {
				t.Fatalf("diagnostic omitted the upgrade command: %q", &diagnostics)
			}
		})
	}
}

func TestReportOutputFormats(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(wire.ServerVersion{Version: version.Client, Protocol: version.LegacyProtocol})
	})
	mux.HandleFunc("POST /api/reports", func(w http.ResponseWriter, r *http.Request) {
		var report model.Report
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(model.Result{ProjectID: 7, Reported: len(report.Tasks)})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"task text", []string{"task", "-id", "work", "-title", "Work"}, "Project ID:     7\nTasks reported: 1\n"},
		{"snapshot text", []string{"snapshot"}, "Project ID:     7\nTasks reported: 0\n"},
		{"snapshot JSON", []string{"snapshot", "-json"}, "{\n  \"project_id\": 7,\n  \"reported\": 0\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			args := append(tc.args, "-addr", address)
			code := (App{}).Run(t.Context(), args, strings.NewReader("[]"), &out, &diagnostics)
			if code != 0 || diagnostics.Len() != 0 || out.String() != tc.want {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
			}
		})
	}
}

func TestNetworkProtocolCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, diagnostic string
		allowOperation             bool
	}{
		{"legacy integer", "", "", true},
		{"patch only", "1.0.9", "", true},
		{"newer minor", "1.1.0", "network protocol version mismatch", true},
		{"incompatible major", "2.0.0", "incompatible network protocol", false},
		{"invalid semantic version", "1.2", "server protocol version unavailable or invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					json.NewEncoder(w).Encode(wire.ServerVersion{
						Version: version.Client, Protocol: version.LegacyProtocol, ProtocolSemver: tc.protocol,
					})
					return
				}
				if !tc.allowOperation || r.URL.Path != "/api/reports" {
					t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				json.NewEncoder(w).Encode(model.Result{ProjectID: 7, Reported: 1})
			}))
			defer service.Close()
			address := strings.TrimPrefix(service.URL, "http://")
			commands := [][]string{{"task", "-addr", address, "-id", "task", "-title", "Work", "-json"}}
			if !tc.allowOperation {
				commands = append(commands, []string{"project", "ls", "-addr", address, "-json"}, []string{"serve", "-stop", "-addr", address})
			}
			for _, command := range commands {
				var out, diagnostics bytes.Buffer
				exit := (App{}).Run(t.Context(), command, nil, &out, &diagnostics)
				if tc.allowOperation && (exit != 0 || !json.Valid(out.Bytes())) || !tc.allowOperation && (exit == 0 || out.Len() != 0) {
					t.Fatalf("%v: exit=%d stdout=%q stderr=%q", command, exit, &out, &diagnostics)
				}
				if tc.diagnostic == "" && diagnostics.Len() != 0 || !strings.Contains(diagnostics.String(), tc.diagnostic) {
					t.Fatalf("%v: expected diagnostic %q, got %q", command, tc.diagnostic, &diagnostics)
				}
				if tc.protocol == "2.0.0" && !strings.Contains(diagnostics.String(), "brew upgrade simonski/backlog/backlog") {
					t.Fatalf("%v: missing client upgrade command in %q", command, &diagnostics)
				}
			}
		})
	}
}
