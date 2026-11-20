package cli

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"harness.engineering/internal/model"
	"harness.engineering/internal/server"
	"harness.engineering/internal/storage"
)

func TestTicketCommands(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	store, err := storage.Open(t.Context(), filepath.Join(project, "backlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := httptest.NewServer(server.Handler(store, "test"))
	defer service.Close()
	address := strings.TrimPrefix(service.URL, "http://")
	run := func(input string, args ...string) (string, string, int) {
		t.Helper()
		var out, diagnostics bytes.Buffer
		args = append(args, "-addr", address)
		code := (App{}).Run(t.Context(), args, strings.NewReader(input), &out, &diagnostics)
		return out.String(), diagnostics.String(), code
	}
	succeed := func(input string, args ...string) string {
		t.Helper()
		out, diagnostics, code := run(input, args...)
		if code != 0 || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
		return out
	}
	list := func(ids string) []model.Task {
		t.Helper()
		out := succeed("", "ls", "-id", ids, "-json")
		var tasks []model.Task
		if err := json.Unmarshal([]byte(out), &tasks); err != nil {
			t.Fatalf("invalid task list %q: %v", out, err)
		}
		return tasks
	}

	if out := succeed("", "ls", "-json"); out != "[]\n" {
		t.Fatalf("empty task list: %q", out)
	}
	succeed(`[{"id":"12","title":"Review"},{"id":"qa&release","title":"Release"},{"id":"15","title":"Triage"}]`, "snapshot")
	succeed(`[{"id":"12"},{"id":"qa&release"}]`, "snapshot")
	if out := succeed("", "ls"); !strings.Contains(out, "qa&release") || !strings.Contains(out, "rejected") {
		t.Fatalf("unexpected human-readable tickets: %q", out)
	}
	succeed("", "update", "-id", "12, 15", "-description", "Sprint work", "-progress", "0")
	succeed("", "block", "-id", "12", "-blocker", "Waiting for QA")
	if tasks := list("12"); len(tasks) != 1 || tasks[0].Status != model.Blocked {
		t.Fatalf("unexpected blocked ticket: %+v", tasks)
	}
	for _, status := range []string{"todo", "doing", "done"} {
		out := succeed("", status, "-id", "12,15", "-json")
		var receipt model.Result
		if err := json.Unmarshal([]byte(out), &receipt); err != nil || receipt.Reported != 2 {
			t.Fatalf("invalid %s receipt: %q", status, out)
		}
		for _, task := range list("15,12") {
			if task.Status != status || task.Omitted || task.Description != "Sprint work" || task.Progress == nil || *task.Progress != 0 {
				t.Fatalf("unexpected ticket after %s: %+v", status, task)
			}
		}
	}
	for _, args := range [][]string{
		{"update", "-id", "12,unknown", "-title", "Replacement"},
		{"todo", "-id", "12,12"},
		{"doing", "-id", "12,"},
		{"block"},
		{"update", "-id", "12"},
		{"ls", "-id", "unknown"},
	} {
		if out, diagnostics, code := run("", args...); code == 0 || out != "" || diagnostics == "" {
			t.Fatalf("%v should fail: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	tasks := list("15,12,qa&release")
	if len(tasks) != 3 || tasks[0].ID != "15" || tasks[0].Status != model.Done ||
		tasks[1].ID != "12" || tasks[1].Status != model.Done || tasks[1].Title != "Review" ||
		tasks[1].Blocker != "Waiting for QA" || tasks[2].ID != "qa&release" || tasks[2].Status != model.Todo {
		t.Fatalf("unexpected final tickets: %+v", tasks)
	}
}
