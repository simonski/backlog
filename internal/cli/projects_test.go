package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"harness.engineering/internal/model"
	"harness.engineering/internal/server"
	"harness.engineering/internal/storage"
	"harness.engineering/internal/version"
)

func TestProjectListing(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(t.Context(), filepath.Join(root, "backlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	report := func(project, id, state string) int64 {
		t.Helper()
		result, err := store.Apply(t.Context(), model.Report{
			Root: filepath.Join(root, project), Tasks: []model.TaskUpdate{{ID: id, Title: &id, Status: &state}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.ProjectID
	}
	active := report("active", "doing-older", model.Doing)
	report("active", "doing-newer", model.Doing)
	report("active", "todo-newest", model.Todo)
	report("idle-old", "other", model.Done)
	idle := report("idle-new", "other-new", model.Done)
	handler := server.Handler(store, "test")
	local := httptest.NewServer(handler)
	defer local.Close()
	t.Setenv("BACKLOG_URL", local.URL)
	run := func(args ...string) string {
		t.Helper()
		var out, diagnostics bytes.Buffer
		code := (App{}).Run(t.Context(), args, strings.NewReader(""), &out, &diagnostics)
		if code != 0 || strings.TrimSpace(diagnostics.String()) != version.Warning(version.Client, version.Server) {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out.String(), diagnostics.String())
		}
		return out.String()
	}
	tickets := func(args ...string) []string {
		t.Helper()
		out := run(append([]string{"ls", "-json"}, args...)...)
		var rows []model.Task
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return ids
	}

	var usage bytes.Buffer
	if code := (App{}).Run(t.Context(), []string{"project"}, nil, &usage, &usage); code != 0 || !strings.Contains(usage.String(), "ls") {
		t.Fatalf("project usage exit=%d output=%q", code, usage.String())
	}
	t.Chdir(root)
	var projects []model.ProjectListing
	if err := json.Unmarshal([]byte(run("project", "ls", "-json")), &projects); err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 || projects[0].ID != active || projects[1].ID != idle {
		t.Fatalf("project ordering: %+v", projects)
	}
	if ids := tickets(); !slices.Equal(ids, []string{"doing-newer", "doing-older", "todo-newest", "other-new", "other"}) {
		t.Fatalf("ticket ordering: %v", ids)
	}

	nested := filepath.Join(root, "active", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	if ids := tickets(); len(ids) != 3 {
		t.Fatalf("path inference: %v", ids)
	}
	if ids := tickets("-project", strconv.FormatInt(idle, 10)); !slices.Equal(ids, []string{"other-new"}) {
		t.Fatalf("explicit project: %v", ids)
	}
	if _, err := exec.LookPath("git"); err == nil {
		checkout := filepath.Join(root, "renamed-checkout")
		for _, args := range [][]string{{"init", "--quiet", checkout}, {"-C", checkout, "remote", "add", "origin", "git@example.net:organisation/active.git"}} {
			if output, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, output)
			}
		}
		t.Chdir(checkout)
		if ids := tickets(); len(ids) != 3 {
			t.Fatalf("Git repository name inference: %v", ids)
		}
	}

	if err := store.SetArchived(t.Context(), idle, true); err != nil {
		t.Fatal(err)
	}
	if err := store.ArchiveTicket(t.Context(), active, "todo-newest"); err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Host = "backlog.example.ts.net"
		handler.ServeHTTP(w, req)
	}))
	defer remote.Close()
	t.Setenv("BACKLOG_URL", remote.URL)
	t.Chdir(root)
	if ids := tickets(); !slices.Equal(ids, []string{"doing-newer", "doing-older", "other"}) {
		t.Fatalf("remote ticket visibility: %v", ids)
	}
	if output := run("project", "ls", "-json"); strings.Contains(output, root) || strings.Contains(output, "idle-new") {
		t.Fatalf("remote project visibility: %s", output)
	}
}

func TestProjectInitUsesGitHubOrPortablePath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("APPDATA", root)
	t.Setenv("BACKLOG_URL", "")
	store, err := storage.Open(t.Context(), filepath.Join(root, "backlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	remote := httptest.NewServer(server.Handler(store, "test"))
	defer remote.Close()
	address := strings.TrimPrefix(remote.URL, "http://")
	run := func(args ...string) []byte {
		t.Helper()
		var out, diagnostics bytes.Buffer
		code := (App{}).Run(t.Context(), append(args, "-addr", address, "-json"), nil, &out, &diagnostics)
		if code != 0 {
			t.Fatalf("%v: exit=%d stderr=%q", args, code, diagnostics.String())
		}
		return out.Bytes()
	}
	init := func() model.ProjectInitResult {
		t.Helper()
		var receipt model.ProjectInitResult
		if err := json.Unmarshal(run("project", "init"), &receipt); err != nil {
			t.Fatal(err)
		}
		return receipt
	}

	local := filepath.Join(root, "notes")
	if err := os.Mkdir(local, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(local)
	if receipt := init(); receipt.Identifier != "~/notes" {
		t.Fatalf("path identifier: %+v", receipt)
	}
	run("task", "-id", "notes-only", "-title", "Notes only")
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	makeRepo := func(name, url string) {
		t.Helper()
		checkout := filepath.Join(root, name)
		for _, args := range [][]string{{"init", "--quiet", checkout}, {"-C", checkout, "remote", "add", "origin", url}} {
			if output, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, output)
			}
		}
		t.Chdir(checkout)
	}
	makeRepo("first-checkout", "git@github.com:Example/repo.git")
	project := init()
	if project.Identifier != "https://github.com/Example/repo" {
		t.Fatalf("repo identifier: %+v", project)
	}
	run("task", "-id", "repository-only", "-title", "Repository only")
	makeRepo("second-checkout", "https://ignored-secret@github.com/example/repo.git")
	var tasks []model.Task
	if err := json.Unmarshal(run("ls"), &tasks); err != nil || len(tasks) != 1 || tasks[0].ID != "repository-only" {
		t.Fatalf("GitHub inference chose the wrong project: %+v (%v)", tasks, err)
	}
	if alias := init(); alias.ProjectID != project.ProjectID || strings.Contains(alias.Identifier, "ignored-secret") {
		t.Fatalf("aliased identifier: %+v", alias)
	}
	var report model.Result
	if err := json.Unmarshal(run("task", "-id", "new", "-title", "New ticket"), &report); err != nil || report.ProjectID != project.ProjectID {
		t.Fatalf("report did not resolve the alias: %+v (%v)", report, err)
	}
}
