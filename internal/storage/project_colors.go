package storage

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"harness.engineering/internal/model"
)

func (store *Store) SetProjectColor(ctx context.Context, projectID int64, preset string) error {
	if !slices.ContainsFunc(model.ProjectColorPresets, func(color model.ProjectColorPreset) bool { return color.Key == preset }) {
		return model.Invalid("unknown project color")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE id = ? AND archived = 0", projectID).Scan(&projectID); err != nil {
		return err
	}
	var previousOwner int64
	if err := tx.QueryRowContext(ctx, "SELECT project_id FROM project_colors WHERE preset = ?", preset).Scan(&previousOwner); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if previousOwner == projectID {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_colors WHERE project_id = ? OR preset = ?", projectID, preset); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO project_colors (project_id, preset) VALUES (?, ?)", projectID, preset); err != nil {
		return err
	}
	if previousOwner != 0 {
		// If every preset is occupied, the displaced project keeps its unique automatic color.
		for _, candidate := range model.ProjectColorPresets {
			picked, err := tx.ExecContext(ctx, `
				INSERT INTO project_colors (project_id, preset) VALUES (?, ?)
				ON CONFLICT (preset) DO NOTHING
			`, previousOwner, candidate.Key)
			if err != nil {
				return err
			}
			count, err := picked.RowsAffected()
			if err != nil {
				return err
			}
			if count > 0 {
				break
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", time.Now().UnixNano(), projectID); err != nil {
		return err
	}
	return tx.Commit()
}
