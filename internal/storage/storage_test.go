package storage

import (
	"errors"
	"path/filepath"
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
	before, _, err := store.Dashboard(t.Context(), initial.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	rename := "Replacement title"
	_, err = store.Apply(t.Context(), model.Report{
		Root:     root,
		Snapshot: true,
		Tasks:    []model.TaskUpdate{{ID: "original", Title: &rename}, {ID: "missing-title"}},
	})
	var invalid model.Invalid
	if !errors.As(err, &invalid) {
		t.Fatalf("expected validation error, got %v", err)
	}

	projects, tasks, err := store.Dashboard(t.Context(), initial.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != original || tasks[0].Omitted {
		t.Fatalf("invalid snapshot changed tasks: %+v", tasks)
	}
	if projects[0].LatestReport != before[0].LatestReport {
		t.Fatal("invalid snapshot advanced the latest report time")
	}

	_, err = store.Apply(t.Context(), model.Report{
		Root:  t.TempDir(),
		Tasks: []model.TaskUpdate{{ID: "missing-title"}},
	})
	if !errors.As(err, &invalid) {
		t.Fatalf("expected validation error, got %v", err)
	}
	projects, _, err = store.Dashboard(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("invalid first report created a project: %+v", projects)
	}
}
