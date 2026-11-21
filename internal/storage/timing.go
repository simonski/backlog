package storage

import (
	"context"
	"database/sql"
)

// Sync against the final ticket state, after a snapshot has restored its included tickets.
func syncTaskTiming(ctx context.Context, tx *sql.Tx, projectID, now int64) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_timing AS tm
		SET active_nanos = active_nanos + MAX(0, ? - started_at), started_at = NULL
		WHERE tm.project_id = ? AND tm.started_at IS NOT NULL AND EXISTS (
			SELECT 1 FROM tasks t WHERE t.project_id = tm.project_id AND t.id = tm.task_id
			AND (t.status <> 'doing' OR t.omitted = 1 OR EXISTS (
				SELECT 1 FROM archived_tasks ar WHERE ar.project_id = t.project_id AND ar.task_id = t.id
			))
		)
	`, now, projectID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_timing (project_id, task_id, started_at)
	SELECT t.project_id, t.id, ? FROM tasks t WHERE t.project_id = ? AND t.status = 'doing' AND t.omitted = 0
	AND NOT EXISTS (SELECT 1 FROM archived_tasks ar WHERE ar.project_id = t.project_id AND ar.task_id = t.id)
		ON CONFLICT (project_id, task_id) DO UPDATE SET started_at = excluded.started_at
		WHERE task_timing.started_at IS NULL
	`, now, projectID)
	return err
}
