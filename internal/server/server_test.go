package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"harness.engineering/internal/model"
	"harness.engineering/internal/storage"
)

func TestRemoteDashboardIsReadOnly(t *testing.T) {
	store, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	title := "Check dashboard access"
	if _, err := store.Apply(t.Context(), model.Report{
		Root:  t.TempDir(),
		Tasks: []model.TaskUpdate{{ID: "check", Title: &title}},
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		method string
		target string
		peer   string
		status int
		edit   bool
	}{
		{"remote IP", "GET", "http://100.75.0.1:15000/projects/1", "100.75.0.2:43210", http.StatusOK, false},
		{"tailnet hostname", "GET", "http://macmini.example.ts.net:15000/projects/1", "100.75.0.2:43210", http.StatusOK, false},
		{"local dashboard", "GET", "http://127.0.0.1:15000/projects/1", "127.0.0.1:43210", http.StatusOK, true},
		{"unexpected hostname", "GET", "http://example.org/projects/1", "127.0.0.1:43210", http.StatusForbidden, false},
		{"remote report", "POST", "http://100.75.0.1:15000/api/reports", "100.75.0.2:43210", http.StatusForbidden, false},
		{"spoofed host", "POST", "http://127.0.0.1:15000/projects/1/archive", "100.75.0.2:43210", http.StatusForbidden, false},
	}
	handler := Handler(store, "test-build")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.target, nil)
			request.RemoteAddr = tc.peer
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != tc.status {
				t.Fatalf("got status %d, want %d", response.Code, tc.status)
			}
			body := response.Body.String()
			if tc.status == http.StatusOK && !strings.Contains(body, title) {
				t.Fatal("task missing from dashboard")
			}
			if strings.Contains(body, "Archive project") != tc.edit {
				t.Fatal("incorrect archive controls")
			}
		})
	}
}
