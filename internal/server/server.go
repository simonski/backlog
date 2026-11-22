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
	"net/http"
	"net/netip"
	"net/url"
	"os"
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
	"reportTime":   func(nanos int64) time.Time { return time.Unix(0, nanos).UTC() },
	"elapsedTime":  elapsedTime,
	"milliseconds": func(nanos int64) int64 { return nanos / int64(time.Millisecond) },
	"documentPath": documentPath,
	"ticketKey":    url.QueryEscape,
}).ParseFS(files, "web/dashboard.html"))

type server struct {
	store         *storage.Store
	buildID       string
	archiveAccess TailscaleArchiveAccess
}

func Handler(store *storage.Store, buildID string) http.Handler {
	return HandlerWithArchiveAccess(store, buildID, TailscaleArchiveAccess{})
}

func HandlerWithArchiveAccess(store *storage.Store, buildID string, access TailscaleArchiveAccess) http.Handler {
	return HandlerWithShutdown(store, buildID, access, nil)
}

func HandlerWithShutdown(store *storage.Store, buildID string, access TailscaleArchiveAccess, shutdown func()) http.Handler {
	app := server{store: store, buildID: buildID, archiveAccess: access}
	assets, err := fs.Sub(files, "web/assets")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	static := http.FileServerFS(assets)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", static))
	mux.Handle("GET /favicon.ico", static)
	mux.HandleFunc("GET /api/version", app.version)
	if shutdown != nil {
		mux.HandleFunc("POST /api/stop", requireAccess(localRequest, func(w http.ResponseWriter, r *http.Request) {
			if !readJSONRequest(w, r, &struct{}{}) {
				return
			}
			writeJSON(w, http.StatusOK, wire.StopResult{PID: os.Getpid()})
			shutdown()
		}))
	}
	mux.HandleFunc("POST /api/reports", requireAccess(localRequest, app.report))
	mux.HandleFunc("GET /api/tasks", app.listTasks)
	mux.HandleFunc("PATCH /api/tasks", requireAccess(localRequest, app.updateTasks))
	mux.HandleFunc("POST /api/documents", requireAccess(localRequest, app.saveDocument))
	mux.HandleFunc("DELETE /api/documents", requireAccess(localRequest, app.clearDocuments))
	mux.HandleFunc("GET /{$}", app.dashboard)
	mux.HandleFunc("GET /projects/{id}", app.dashboard)
	mux.HandleFunc("GET /projects/{id}/docs/{name}", app.document)
	mux.HandleFunc("POST /projects/{id}/archive", requireAccess(app.canArchive, app.archive))
	mux.HandleFunc("POST /projects/{id}/archive/ticket", requireAccess(app.canArchive, app.archiveTicket))
	mux.HandleFunc("POST /projects/{id}/archive/{status}", requireAccess(app.canArchive, app.archiveColumn))
	mux.HandleFunc("POST /projects/{id}/color", requireAccess(app.canArchive, app.setProjectColor))
	handler := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !dashboardHost(requestHost(r)) {
			http.Error(w, "use a local address, an IP address, or a Tailscale hostname", http.StatusForbidden)
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
	return loopbackPeer(r) && local.IsLoopback(requestHost(r))
}

func (app server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, wire.ServerVersion{Version: version.Server, Protocol: version.Protocol})
}

func (app server) report(w http.ResponseWriter, r *http.Request) {
	var report model.Report
	if !readJSONRequest(w, r, &report) {
		return
	}
	result, err := app.store.Apply(r.Context(), report)
	writeAPIResponse(w, result, err, "save report")
}

func (app server) listTasks(w http.ResponseWriter, r *http.Request) {
	if !localRequest(r) {
		writeJSON(w, http.StatusForbidden, wire.Failure{Error: "listing tasks requires a local connection"})
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, wire.Failure{Error: "invalid task query"})
		return
	}
	tasks, err := app.store.ListTasks(r.Context(), query.Get("root"), query["id"])
	writeAPIResponse(w, tasks, err, "list tasks")
}

func (app server) updateTasks(w http.ResponseWriter, r *http.Request) {
	var changes model.TaskChanges
	if !readJSONRequest(w, r, &changes) {
		return
	}
	result, err := app.store.UpdateTasks(r.Context(), changes)
	writeAPIResponse(w, result, err, "update tasks")
}

func readJSONRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, wire.Failure{Error: "expected application/json"})
		return false
	}
	err = wire.ReadJSON(http.MaxBytesReader(w, r.Body, wire.MaxReportBytes), target)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, wire.Failure{Error: "invalid JSON request: " + err.Error()})
		return false
	}
	return true
}

func writeAPIResponse(w http.ResponseWriter, result any, err error, operation string) {
	var invalid model.Invalid
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, wire.Failure{Error: invalid.Error()})
	case err != nil:
		log.Printf("%s: %v", operation, err)
		writeJSON(w, http.StatusInternalServerError, wire.Failure{Error: "could not " + operation})
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

func (group group) ArchiveStatus() string {
	switch group.Status {
	case "omitted":
		return model.Rejected
	case model.Doing:
		return ""
	default:
		return group.Status
	}
}

type page struct {
	Title        string
	Section      string
	Projects     []model.Project
	Selected     *model.Project
	Groups       []group
	Documents    []string
	Activity     activityChart
	CanEdit      bool
	ColorPresets []model.ProjectColorPreset
	BuildID      string
	Version      string
}

func (app server) dashboard(w http.ResponseWriter, r *http.Request) {
	view := page{
		Title: "Projects", Section: "projects", CanEdit: app.canArchive(r),
		BuildID: app.buildID, Version: version.Server, ColorPresets: model.ProjectColorPresets,
	}
	var selectedID int64
	if r.PathValue("id") != "" {
		id, err := projectID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		selectedID = id
		view.Section = "project"
	}
	var projects []model.Project
	var tasks []model.Task
	var activity []model.PeriodActivity
	var oldest string
	var err error
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	selectedRange := findActivityRange(r.URL.Query().Get("range"))
	if view.Section == "projects" {
		projects, activity, oldest, err = app.store.Homepage(r.Context(), selectedRange.first(today).Format(time.DateOnly), today.Format(time.DateOnly), selectedRange.step)
		if err == nil && !selectedRange.available(today, oldest) {
			selectedRange = findActivityRange(defaultActivityRange)
			projects, activity, oldest, err = app.store.Homepage(r.Context(), selectedRange.first(today).Format(time.DateOnly), today.Format(time.DateOnly), selectedRange.step)
		}
	} else {
		projects, tasks, view.Documents, err = app.store.Dashboard(r.Context(), selectedID)
	}
	if err != nil {
		pageError(w, r, err)
		return
	}

	for _, project := range projects {
		if project.Archived {
			continue
		}
		if project.ID == selectedID {
			view.Selected = &project
			view.Title = project.Name()
		}
		view.Projects = append(view.Projects, project)
	}
	if view.Selected != nil {
		view.Groups = taskGroups(tasks)
	}
	if view.Section == "projects" {
		view.Activity = makeActivityChart(today, selectedRange, oldest, view.Projects, activity)
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
		{Status: model.Blocked, Label: "Blocked"},
		{Status: model.Todo, Label: "To do"},
		{Status: model.Doing, Label: "Doing"},
		{Status: model.Done, Label: "Done"},
		{Status: "omitted", Label: "Rejected"},
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
	if value == "true" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/projects/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func archivableStatus(status string) bool {
	return status == model.Blocked || status == model.Todo || status == model.Done || status == model.Rejected
}

func (app server) archiveColumn(w http.ResponseWriter, r *http.Request) {
	id, err := projectID(r)
	status := r.PathValue("status")
	if err != nil || !archivableStatus(status) {
		http.NotFound(w, r)
		return
	}
	if err := app.store.ArchiveColumn(r.Context(), id, status); err != nil {
		pageError(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (app server) archiveTicket(w http.ResponseWriter, r *http.Request) {
	id, err := projectID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// A ticket identifier can be escaped once in the page and again by the form.
	r.Body = http.MaxBytesReader(w, r.Body, wire.MaxReportBytes*5+64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid archive request", http.StatusBadRequest)
		return
	}
	taskID, err := url.QueryUnescape(r.PostForm.Get("ticket_key"))
	if err != nil || strings.TrimSpace(taskID) == "" {
		http.Error(w, "ticket ID is required", http.StatusBadRequest)
		return
	}
	if err := app.store.ArchiveTicket(r.Context(), id, taskID); err != nil {
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
