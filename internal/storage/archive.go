package storage

import (
	"context"
	"time"
)

func (store *Store) ArchiveColumn(ctx context.Context, projectID int64, status string) error {
	return store.archiveTickets(ctx, projectID, "", status)
}

func (store *Store) ArchiveTicket(ctx context.Context, projectID int64, taskID string) error {
	return store.archiveTickets(ctx, projectID, taskID, "")
}

func (store *Store) archiveTickets(ctx context.Context, projectID int64, taskID, status string) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE id = ? AND archived = 0", projectID).Scan(&projectID); err != nil {
		return err
	}
	if taskID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM tasks WHERE project_id = ? AND id = ?", projectID, taskID).Scan(&exists); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO archived_tasks (project_id, task_id)
		SELECT project_id, id FROM tasks WHERE project_id = ?
		AND CASE WHEN ? <> '' THEN id = ?
		         WHEN omitted = 1 THEN 'rejected' = ?
		         ELSE status = ? END
		ON CONFLICT (project_id, task_id) DO NOTHING
	`, projectID, taskID, taskID, status, status)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		now := time.Now().UnixNano()
		if err := syncTaskTiming(ctx, tx, projectID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", now, projectID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
