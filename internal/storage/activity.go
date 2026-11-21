package storage

import (
	"context"
	"database/sql"
	"time"

	"harness.engineering/internal/model"
)

const documentPromotionDuration = 6 * time.Second

// Homepage reads the project list and activity from one consistent view.
func (store *Store) Homepage(ctx context.Context, first, last string, daysPerColumn int) ([]model.Project, []model.PeriodActivity, string, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, "", err
	}
	defer tx.Rollback()
	projects, err := listProjects(ctx, tx, time.Now().Add(-documentPromotionDuration).UnixNano())
	if err != nil {
		return nil, nil, "", err
	}
	var oldest string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MIN(a.day), '')
		FROM daily_task_activity a JOIN projects p ON p.id = a.project_id
		WHERE a.day <= ? AND p.archived = 0
		AND NOT EXISTS (SELECT 1 FROM archived_tasks ar WHERE ar.project_id = a.project_id AND ar.task_id = a.task_id)
	`, last).Scan(&oldest); err != nil {
		return nil, nil, "", err
	}
	rows, err := tx.QueryContext(ctx, `
		WITH visible AS (
			SELECT a.day, a.project_id, a.task_id, a.status,
				CAST((julianday(a.day) - julianday(?1)) / ?3 AS INTEGER) AS bucket
			FROM daily_task_activity a JOIN projects p ON p.id = a.project_id
			WHERE a.day BETWEEN ?1 AND ?2 AND p.archived = 0
			AND NOT EXISTS (SELECT 1 FROM archived_tasks ar WHERE ar.project_id = a.project_id AND ar.task_id = a.task_id)
		), ranked AS (
			SELECT bucket, project_id, status,
				ROW_NUMBER() OVER (PARTITION BY bucket, project_id, task_id ORDER BY day DESC) AS latest
			FROM visible
		)
		SELECT date(?1, '+' || (bucket * ?3) || ' days'), project_id, status, COUNT(*)
		FROM ranked WHERE latest = 1 GROUP BY bucket, project_id, status
	`, first, last, daysPerColumn)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()
	var activity []model.PeriodActivity
	for rows.Next() {
		var period model.PeriodActivity
		if err := rows.Scan(&period.Start, &period.ProjectID, &period.Status, &period.Count); err != nil {
			return nil, nil, "", err
		}
		activity = append(activity, period)
	}
	return projects, activity, oldest, rows.Err()
}
