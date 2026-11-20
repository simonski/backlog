package storage

import (
	"context"
	"database/sql"

	"harness.engineering/internal/model"
)

// Homepage reads the project list and activity from one consistent view.
func (store *Store) Homepage(ctx context.Context, first, last string) ([]model.Project, []model.DailyActivity, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	projects, err := listProjects(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT a.day, a.project_id, a.status, COUNT(*)
		FROM daily_task_activity a JOIN projects p ON p.id = a.project_id
		WHERE a.day BETWEEN ? AND ? AND p.archived = 0
		GROUP BY a.day, a.project_id, a.status
	`, first, last)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var activity []model.DailyActivity
	for rows.Next() {
		var day model.DailyActivity
		if err := rows.Scan(&day.Day, &day.ProjectID, &day.Status, &day.Count); err != nil {
			return nil, nil, err
		}
		activity = append(activity, day)
	}
	return projects, activity, rows.Err()
}
