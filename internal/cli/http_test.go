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
				json.NewEncoder(w).Encode(wire.ServerVersion{Version: tc.serverVersion})
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
			code := (App{}).Run(t.Context(), []string{"task", "-addr", address, "-id", "task", "-title", "Work"}, nil, &out, &diagnostics)
			if len(requests) != 2 || <-requests != "/api/version" || <-requests != "/api/reports" {
				t.Fatal("expected a version request followed by the report")
			}
			switch {
			case tc.resultCode == http.StatusOK:
				var result model.Result
				if err := json.Unmarshal(out.Bytes(), &result); code != 0 || err != nil || result.Reported != 1 {
					t.Fatalf("invalid success receipt: exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
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
		})
	}
}
