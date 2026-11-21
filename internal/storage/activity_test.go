package storage

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"harness.engineering/internal/model"
)

func TestHomepageActivityUsesLatestStatusInEachWeek(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	store, err := Open(ctx, filepath.Join(root, "backlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	title := "Work"
	receipt, err := store.Apply(ctx, model.Report{Root: root, Tasks: []model.TaskUpdate{{ID: "work", Title: &title}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO daily_task_activity (day, project_id, task_id, status) VALUES
		('2020-05-01', ?1, 'work', 'todo'), ('2020-05-03', ?1, 'work', 'doing'), ('2020-05-08', ?1, 'work', 'done')
	`, receipt.ProjectID); err != nil {
		t.Fatal(err)
	}
	_, activity, oldest, err := store.Homepage(ctx, "2020-05-01", "2020-05-14", 7)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.PeriodActivity{
		{Start: "2020-05-01", ProjectID: receipt.ProjectID, Status: model.Doing, Count: 1},
		{Start: "2020-05-08", ProjectID: receipt.ProjectID, Status: model.Done, Count: 1},
	}
	slices.SortFunc(activity, func(a, b model.PeriodActivity) int { return strings.Compare(a.Start, b.Start) })
	if oldest != "2020-05-01" || !slices.Equal(activity, want) {
		t.Fatalf("oldest %q, weekly counts %+v", oldest, activity)
	}
}
