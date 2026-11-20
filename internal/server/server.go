package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
	"harness.engineering/internal/storage"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

//go:embed web
var files embed.FS

var dashboard = template.Must(template.New("dashboard.html").Funcs(template.FuncMap{
	"reportTime": func(nanos int64) time.Time { return time.Unix(0, nanos).UTC() },
}).ParseFS(files, "web/dashboard.html"))

type server struct {
	store   *storage.Store
	buildID string
}

func Handler(store *storage.Store, buildID string) http.Handler {
	app := server{store: store, buildID: buildID}
	assets, err := fs.Sub(files, "web/assets")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /api/version", app.version)
	mux.HandleFunc("POST /api/reports", app.report)
	mux.HandleFunc("GET /{$}", app.dashboard)
	mux.HandleFunc("GET /archived", app.dashboard)
	mux.HandleFunc("GET /projects/{id}", app.dashboard)
	mux.HandleFunc("POST /projects/{id}/archive", app.archive)
	handler := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !dashboardHost(requestHost(r)) {
			http.Error(w, "use a local address, an IP address, or a Tailscale hostname", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !localRequest(r) {
			http.Error(w, "project changes require a local connection", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		handler.ServeHTTP(w, r)
	})
}

func requestHost(r *http.Request) string {
	return (&url.URL{Host: r.Host}).Hostname()
}

func dashboardHost(host string) bool {
	_, ipErr := netip.ParseAddr(host)
	dnsHost := strings.ToLower(strings.TrimSuffix(host, "."))
	return local.IsLoopback(host) || ipErr == nil || strings.HasSuffix(dnsHost, ".ts.net")
}

func localRequest(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && local.IsLoopback(peer) && local.IsLoopback(requestHost(r))
}

func (app server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, wire.ServerVersion{Version: version.Server, Protocol: version.Protocol})
}

func (app server) report(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, wire.Failure{Error: "expected application/json"})
		return
	}
	var report model.Report
	err = wire.ReadJSON(http.MaxBytesReader(w, r.Body, wire.MaxReportBytes), &report)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, wire.Failure{Error: "invalid report: " + err.Error()})
		return
	}
	result, err := app.store.Apply(r.Context(), report)
	var invalid model.Invalid
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, wire.Failure{Error: invalid.Error()})
	case err != nil:
		log.Printf("save report: %v", err)
		writeJSON(w, http.StatusInternalServerError, wire.Failure{Error: "could not save report"})
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}

type group struct {
	Status string
	Label  string
	Tasks  []model.Task
}

type page struct {
	Title    string
	Section  string
	Projects []model.Project
	Archived []model.Project
	Selected *model.Project
	Groups   []group
	CanEdit  bool
	BuildID  string
	Version  string
}

func (app server) dashboard(w http.ResponseWriter, r *http.Request) {
	view := page{
		Title: "Projects", Section: "projects", CanEdit: localRequest(r),
		BuildID: app.buildID, Version: version.Server,
	}
	var selectedID int64
	switch {
	case r.URL.Path == "/archived":
		view.Title, view.Section = "Archived projects", "archived"
	case r.PathValue("id") != "":
		id, err := projectID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		selectedID = id
		view.Section = "project"
	}
	projects, tasks, err := app.store.Dashboard(r.Context(), selectedID)
	if err != nil {
		pageError(w, r, err)
		return
	}

	for _, project := range projects {
		if project.ID == selectedID {
			view.Selected = &project
			view.Title = project.Name()
		}
		if project.Archived {
			view.Archived = append(view.Archived, project)
		} else {
			view.Projects = append(view.Projects, project)
		}
	}
	if view.Selected != nil {
		view.Groups = taskGroups(tasks)
	}

	var output bytes.Buffer
	if err := dashboard.Execute(&output, view); err != nil {
		pageError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := output.WriteTo(w); err != nil {
		log.Printf("write dashboard: %v", err)
	}
}

func taskGroups(tasks []model.Task) []group {
	groups := []group{
		{Status: model.Doing, Label: "Doing"},
		{Status: model.Blocked, Label: "Blocked"},
		{Status: model.Todo, Label: "To do"},
		{Status: model.Done, Label: "Done"},
		{Status: "omitted", Label: "No longer needed"},
	}
	for _, task := range tasks {
		for i := range groups {
			if (task.Omitted && groups[i].Status == "omitted") ||
				(!task.Omitted && groups[i].Status == task.Status) {
				groups[i].Tasks = append(groups[i].Tasks, task)
				break
			}
		}
	}
	return groups
}

func (app server) archive(w http.ResponseWriter, r *http.Request) {
	id, err := projectID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid archive request", http.StatusBadRequest)
		return
	}
	value := r.PostForm.Get("archived")
	if value != "true" && value != "false" {
		http.Error(w, "expected archived=true or archived=false", http.StatusBadRequest)
		return
	}
	if err := app.store.SetArchived(r.Context(), id, value == "true"); err != nil {
		pageError(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func projectID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil && id <= 0 {
		err = errors.New("invalid project ID")
	}
	return id, err
}

func pageError(w http.ResponseWriter, r *http.Request, err error) {
	if storage.IsMissing(err) {
		http.NotFound(w, r)
		return
	}
	log.Printf("dashboard: %v", err)
	http.Error(w, "could not load dashboard", http.StatusInternalServerError)
}
