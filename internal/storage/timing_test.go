package storage

import (
	"path/filepath"
	"testing"
	"time"

	"harness.engineering/internal/model"
)

func TestTaskTimingTracksOnlyDoingAcrossSnapshotsAndRestart(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	path := filepath.Join(root, "backlog.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	apply := func(snapshot bool, updates ...model.TaskUpdate) {
		t.Helper()
		if len(updates) == 0 {
			updates = []model.TaskUpdate{}
		}
		if _, err := store.Apply(ctx, model.Report{Root: root, Snapshot: snapshot, Tasks: updates}); err != nil {
			t.Fatal(err)
		}
	}
	read := func() model.Task {
		t.Helper()
		tasks, err := store.ListTasks(ctx, root, []string{"work"})
		if err != nil || len(tasks) != 1 {
			t.Fatalf("read ticket: %+v (%v)", tasks, err)
		}
		return tasks[0]
	}
	backdate := func(duration time.Duration) int64 {
		t.Helper()
		start := time.Now().Add(-duration).UnixNano()
		if _, err := store.db.ExecContext(ctx, "UPDATE task_timing SET started_at = ?", start); err != nil {
			t.Fatal(err)
		}
		return start
	}
	title, doing := "Work", model.Doing
	apply(false, model.TaskUpdate{ID: "work", Title: &title, Status: &doing})
	start := backdate(3 * time.Second)
	apply(true, model.TaskUpdate{ID: "work"})
	if task := read(); task.StartedAt == nil || *task.StartedAt != start {
		t.Fatalf("snapshot restarted a running timer: %+v", task)
	}
	blocked := model.Blocked
	apply(false, model.TaskUpdate{ID: "work", Status: &blocked})
	paused := read()
	if paused.StartedAt != nil || paused.ActiveNanos == nil || *paused.ActiveNanos < int64(3*time.Second) {
		t.Fatalf("timer did not pause when blocked: %+v", paused)
	}
	apply(false, model.TaskUpdate{ID: "work", Status: &doing})
	resumed := read()
	if resumed.StartedAt == nil || *resumed.StartedAt <= start || *resumed.ActiveNanos != *paused.ActiveNanos {
		t.Fatalf("timer did not resume from the saved total: %+v", resumed)
	}
	backdate(2 * time.Second)
	apply(true)
	rejected := read()
	if !rejected.Omitted || rejected.StartedAt != nil || *rejected.ActiveNanos < *paused.ActiveNanos+int64(2*time.Second) {
		t.Fatalf("omitting the ticket did not pause the timer: %+v", rejected)
	}
	apply(false, model.TaskUpdate{ID: "work", Status: &doing})
	active := read()
	if active.StartedAt == nil {
		t.Fatalf("timer did not resume after rejection: %+v", active)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if task := read(); task.StartedAt == nil || *task.StartedAt != *active.StartedAt {
		t.Fatalf("restart changed the running timer: %+v", task)
	}
	done := model.Done
	apply(false, model.TaskUpdate{ID: "work", Status: &done})
	finished := read()
	if finished.StartedAt != nil || *finished.ActiveNanos < *rejected.ActiveNanos {
		t.Fatalf("completion did not freeze the total: %+v", finished)
	}
	apply(false, model.TaskUpdate{ID: "work", Status: &done})
	if task := read(); task.StartedAt != nil || *task.ActiveNanos != *finished.ActiveNanos {
		t.Fatalf("repeated Done report changed the total: %+v", task)
	}
}
