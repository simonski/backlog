package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harness.engineering/internal/model"
	"harness.engineering/internal/storage"
)

func TestClientPresenceOnHomepage(t *testing.T) {
	store, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "backlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, title := model.Doing, "Working on it"
	if _, err := store.Apply(t.Context(), model.Report{
		Root: filepath.Join(t.TempDir(), "active"), Tasks: []model.TaskUpdate{{ID: "1", Title: &title, Status: &status}},
	}); err != nil {
		t.Fatal(err)
	}
	handler := Handler(store, "test")
	send := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "https://backlog.example.ts.net"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	heartbeat := send(http.MethodPost, "/api/client", `{"username":"Simon <mini>"}`)
	var snapshot struct {
		Projects []model.ProjectListing `json:"projects"`
	}
	if heartbeat.Code != http.StatusOK || json.Unmarshal(heartbeat.Body.Bytes(), &snapshot) != nil ||
		len(snapshot.Projects) != 1 || snapshot.Projects[0].Doing != 1 || snapshot.Projects[0].Root != "" {
		t.Fatalf("heartbeat: %d %s", heartbeat.Code, heartbeat.Body)
	}
	if html := send(http.MethodGet, "/", "").Body.String(); !strings.Contains(html, "Simon &lt;mini&gt;") || strings.Contains(html, "Simon <mini>") {
		t.Fatal("homepage omitted or failed to escape the monitoring name")
	}
	if response := send(http.MethodPost, "/api/client", `{"username":"invalid\u0000name"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid username: %d %s", response.Code, response.Body)
	}

	var presence monitorPresence
	now := time.Now()
	presence.heartbeat("first", now)
	presence.heartbeat("first", now.Add(time.Second))
	if names := presence.active(now.Add(monitorTimeout)); len(names) != 1 || names[0] != "first" {
		t.Fatalf("active names: %v", names)
	}
	if names := presence.active(now.Add(monitorTimeout + time.Second)); len(names) != 0 {
		t.Fatalf("expired names: %v", names)
	}
}
