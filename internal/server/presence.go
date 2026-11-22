package server

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

const monitorTimeout = 35 * time.Second
const maxMonitorNames = 256

type monitorPresence struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (presence *monitorPresence) prune(now time.Time) {
	for name, seen := range presence.seen {
		if now.Sub(seen) >= monitorTimeout {
			delete(presence.seen, name)
		}
	}
}

func (presence *monitorPresence) heartbeat(name string, now time.Time) bool {
	presence.mu.Lock()
	defer presence.mu.Unlock()
	presence.prune(now)
	if _, seen := presence.seen[name]; !seen && len(presence.seen) >= maxMonitorNames {
		return false
	}
	if presence.seen == nil {
		presence.seen = make(map[string]time.Time)
	}
	presence.seen[name] = now
	return true
}

func (presence *monitorPresence) active(now time.Time) []string {
	presence.mu.Lock()
	defer presence.mu.Unlock()
	presence.prune(now)
	names := make([]string, 0, len(presence.seen))
	for name := range presence.seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (app server) monitorClient(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var heartbeat wire.ClientHeartbeat
	if !readJSONRequest(w, r, &heartbeat) {
		return
	}
	name := strings.TrimSpace(heartbeat.Username)
	if !model.ValidMonitorName(name) {
		writeAPIResponse(w, nil, model.Invalid("username must be at most 48 printable characters"), "update client presence")
		return
	}
	projects, err := app.store.ListProjects(r.Context(), model.ProjectContext{}, false, false, nil)
	if err != nil {
		writeAPIResponse(w, nil, err, "list projects")
		return
	}
	if name != "" && !app.presence.heartbeat(name, time.Now()) {
		writeJSON(w, http.StatusTooManyRequests, wire.Failure{Error: "too many monitoring usernames"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ServerVersion string                 `json:"server_version"`
		Projects      []model.ProjectListing `json:"projects"`
	}{version.Server, projects})
}
