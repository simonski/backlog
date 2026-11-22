package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"harness.engineering/internal/model"
	"harness.engineering/internal/storage"
)

func TestDashboardArchiveControlsAndRemoteRestrictions(t *testing.T) {
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
		name    string
		method  string
		target  string
		peer    string
		status  int
		archive bool
	}{
		{"remote IP", "GET", "http://100.75.0.1:15000/projects/1", "100.75.0.2:43210", http.StatusOK, true},
		{"tailnet hostname", "GET", "http://macmini.example.ts.net:15000/projects/1", "100.75.0.2:43210", http.StatusOK, true},
		{"local dashboard", "GET", "http://127.0.0.1:15000/projects/1", "127.0.0.1:43210", http.StatusOK, true},
		{"unexpected hostname", "GET", "http://example.org/projects/1", "127.0.0.1:43210", http.StatusForbidden, false},
		{"remote report", "POST", "http://100.75.0.1:15000/api/reports", "100.75.0.2:43210", http.StatusForbidden, false},
		{"remote document upload", "POST", "http://100.75.0.1:15000/api/documents", "100.75.0.2:43210", http.StatusForbidden, false},
		{"remote archive column", "POST", "http://100.75.0.1:15000/projects/1/archive/blocked", "100.75.0.2:43210", http.StatusSeeOther, false},
		{"unsupported archive column", "POST", "http://127.0.0.1:15000/projects/1/archive/doing", "127.0.0.1:43210", http.StatusNotFound, false},
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
			if strings.Contains(body, "Archive project") != tc.archive {
				t.Fatal("incorrect archive controls")
			}
		})
	}
}

func TestShutdownRequiresLocalSameOriginRequest(t *testing.T) {
	stops := 0
	handler := HandlerWithShutdown(nil, "test", TailscaleArchiveAccess{}, func() { stops++ })
	for _, tc := range []struct {
		host, peer, origin, forwarded string
		status                        int
	}{
		{"127.0.0.1", "100.75.0.2:1234", "", "", http.StatusForbidden},
		{"mini.example.ts.net", "127.0.0.1:1234", "", "", http.StatusForbidden},
		{"127.0.0.1", "127.0.0.1:1234", "https://other.example.com", "", http.StatusForbidden},
		{"127.0.0.1", "127.0.0.1:1234", "", "mini.example.ts.net", http.StatusForbidden},
		{"127.0.0.1", "127.0.0.1:1234", "", "", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/stop", strings.NewReader("{}"))
		request.RemoteAddr = tc.peer
		request.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			request.Header.Set("Origin", tc.origin)
		}
		if tc.forwarded != "" {
			request.Header.Set("X-Forwarded-Host", tc.forwarded)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Errorf("host=%s peer=%s origin=%s: got %d, want %d", tc.host, tc.peer, tc.origin, response.Code, tc.status)
		}
	}
	if stops != 1 {
		t.Fatalf("got %d shutdown requests, want one", stops)
	}
}

func TestTailscaleArchiveAndProjectManagementAccess(t *testing.T) {
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
	const host = "mini.example.ts.net:15001"
	const login = "owner@example.com"
	access, err := NewTailscaleArchiveAccess(host, login, "127.0.0.1:15000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTailscaleArchiveAccess(host, login, "0.0.0.0:15000"); err == nil {
		t.Fatal("archive access allowed a non-loopback listener")
	}
	handler := HandlerWithArchiveAccess(store, "test-build", access)
	call := func(method, path string, form url.Values, change func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://"+host+path,
			strings.NewReader(form.Encode()))
		request.RemoteAddr = "127.0.0.1:1234"
		request.Header.Set("X-Forwarded-Host", host)
		request.Header.Set("X-Forwarded-Proto", "https")
		request.Header.Set("Tailscale-User-Login", login)
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", "https://"+host)
		}
		if change != nil {
			change(request)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	noLogin := func(r *http.Request) { r.Header.Del("Tailscale-User-Login") }
	if response := call(http.MethodGet, "/projects/1", nil, noLogin); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `id="archive-todo-button"`) {
		t.Fatal("the dashboard viewer could not see the column archive button")
	}
	if response := call(http.MethodGet, "/", nil, nil); !strings.Contains(response.Body.String(), `data-color-action="/projects/1/color"`) {
		t.Fatal("the allowed identity could not see the project color action")
	}
	if response := call(http.MethodGet, "/", nil, noLogin); !strings.Contains(response.Body.String(), `id="archive-project-1-button"`) ||
		strings.Contains(response.Body.String(), `data-color-action=`) {
		t.Fatal("the dashboard viewer should see project archiving with no color action")
	}
	for _, tc := range []struct {
		name   string
		change func(*http.Request)
	}{
		{"wrong login", func(r *http.Request) { r.Header.Set("Tailscale-User-Login", "other@example.com") }},
		{"no login", noLogin},
		{"non-loopback peer", func(r *http.Request) { r.RemoteAddr = "100.75.0.2:1234" }},
		{"plain HTTP", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") }},
		{"wrong port", func(r *http.Request) { r.Host = "mini.example.ts.net:9999" }},
		{"public route", func(r *http.Request) { r.Header.Set("Tailscale-Funnel-Request", "?1") }},
		{"cross-origin", func(r *http.Request) { r.Header.Set("Origin", "https://other.example.com") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if response := call(http.MethodPost, "/projects/1/color", url.Values{"preset": {"rose"}}, tc.change); response.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", response.Code)
			}
		})
	}
	if response := call(http.MethodPost, "/api/documents", nil, nil); response.Code != http.StatusForbidden {
		t.Fatalf("document upload got %d", response.Code)
	}
	ticket := url.Values{"ticket_key": {"check"}}
	if response := call(http.MethodPost, "/projects/1/archive/ticket", ticket, func(r *http.Request) {
		noLogin(r)
		r.Header.Set("Origin", "https://other.example.com")
	}); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin archive request got %d", response.Code)
	}
	if response := call(http.MethodPost, "/projects/1/archive/ticket", ticket, noLogin); response.Code != http.StatusSeeOther {
		t.Fatalf("archive request got %d: %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodGet, "/projects/1", nil, noLogin); strings.Contains(response.Body.String(), title) {
		t.Fatal("archived ticket still appeared on the board")
	}
	if response := call(http.MethodPost, "/projects/1/archive", url.Values{"archived": {"true"}}, noLogin); response.Code != http.StatusSeeOther {
		t.Fatalf("project archive got %d: %s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/projects/1/archive", url.Values{"archived": {"false"}}, noLogin); response.Code != http.StatusForbidden {
		t.Fatalf("project restore got %d", response.Code)
	}
}
