package storage

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"harness.engineering/internal/model"
)

func TestInvalidSnapshotRollsBackAllChanges(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	root := t.TempDir()
	original := "Original title"
	initial, err := store.Apply(t.Context(), model.Report{
		Root:  root,
		Tasks: []model.TaskUpdate{{ID: "original", Title: &original}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _, err := store.Dashboard(t.Context(), initial.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	_, activityBefore, _, err := store.Homepage(t.Context(), "0001-01-01", "9999-12-31", 1)
	if err != nil || len(activityBefore) != 1 || activityBefore[0].Status != model.Todo {
		t.Fatalf("initial activity: %+v (%v)", activityBefore, err)
	}

	rename := "Replacement title"
	status := model.Doing
	_, err = store.Apply(t.Context(), model.Report{
		Root:     root,
		Snapshot: true,
		Tasks:    []model.TaskUpdate{{ID: "original", Title: &rename, Status: &status}, {ID: "missing-title"}},
	})
	var invalid model.Invalid
	if !errors.As(err, &invalid) {
		t.Fatalf("expected validation error, got %v", err)
	}

	projects, tasks, _, err := store.Dashboard(t.Context(), initial.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != original || tasks[0].Omitted {
		t.Fatalf("invalid snapshot changed tasks: %+v", tasks)
	}
	if projects[0].LatestReport != before[0].LatestReport {
		t.Fatal("invalid snapshot advanced the latest report time")
	}
	_, activityAfter, _, err := store.Homepage(t.Context(), "0001-01-01", "9999-12-31", 1)
	if err != nil || !slices.Equal(activityAfter, activityBefore) {
		t.Fatalf("invalid snapshot changed activity: %+v (%v)", activityAfter, err)
	}

	_, err = store.Apply(t.Context(), model.Report{
		Root:  t.TempDir(),
		Tasks: []model.TaskUpdate{{ID: "missing-title"}},
	})
	if !errors.As(err, &invalid) {
		t.Fatalf("expected validation error, got %v", err)
	}
	projects, _, _, err = store.Dashboard(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("invalid first report created a project: %+v", projects)
	}
}

func TestLifetimeDoneIncludesArchivedAndReopenedTickets(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	done, doing, title := model.Done, model.Doing, "Work"
	result, err := store.Apply(ctx, model.Report{
		Root: root, Snapshot: true,
		Tasks: []model.TaskUpdate{{ID: "archived", Title: &title, Status: &done}, {ID: "reopened", Title: &title, Status: &done}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ArchiveTicket(ctx, result.ProjectID, "archived"); err != nil {
		t.Fatal(err)
	}
	for _, status := range []*string{&doing, &done, &doing} {
		if _, err := store.UpdateTasks(ctx, model.TaskChanges{
			Root: root, Tasks: []model.TaskUpdate{{ID: "reopened", Status: status}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	projects, visible, _, err := store.Dashboard(ctx, result.ProjectID)
	if err != nil || len(projects) != 1 || projects[0].Done != 2 ||
		len(visible) != 1 || visible[0].Status != model.Doing {
		t.Fatalf("homepage totals or board state incorrect: %+v %+v (%v)", projects, visible, err)
	}
}
