package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"harness.engineering/internal/model"
)

func projectForRoot(ctx context.Context, tx *sql.Tx, root string, create bool) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM projects WHERE root = ?
		UNION ALL SELECT project_id FROM project_locations WHERE root = ? LIMIT 1
	`, root, root).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) && create {
		err = tx.QueryRowContext(ctx, "INSERT INTO projects (root) VALUES (?) RETURNING id", root).Scan(&id)
	}
	return id, err
}

func (store *Store) InitProject(ctx context.Context, identity model.ProjectIdentity) (model.ProjectInitResult, error) {
	var result model.ProjectInitResult
	root, err := model.ResolveProjectPath(identity.Path, store.home)
	if err != nil {
		return result, err
	}
	if identity.Repository != "" && model.GitHubRepository(identity.Repository) != identity.Repository {
		return result, model.Invalid("repository must be a GitHub repository URL")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	id, err := projectForRoot(ctx, tx, root, false)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	if identity.Repository != "" {
		var repositoryID int64
		err = tx.QueryRowContext(ctx, "SELECT project_id FROM project_repositories WHERE repository = ?", identity.Repository).Scan(&repositoryID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		if repositoryID != 0 {
			if id != 0 && id != repositoryID {
				return result, model.Invalid("this path and GitHub repository already belong to different projects")
			}
			if id == 0 {
				if _, err := tx.ExecContext(ctx, "INSERT INTO project_locations (root, project_id) VALUES (?, ?)", root, repositoryID); err != nil {
					return result, err
				}
				id = repositoryID
			}
		}
	}
	if id == 0 {
		id, err = projectForRoot(ctx, tx, root, true)
		if err != nil {
			return result, err
		}
	}
	if identity.Repository != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_repositories (project_id, repository) VALUES (?, ?)
			ON CONFLICT (project_id) DO UPDATE SET repository = excluded.repository
		`, id, identity.Repository); err != nil {
			return result, err
		}
	}
	repository := identity.Repository
	if repository == "" {
		err := tx.QueryRowContext(ctx, "SELECT repository FROM project_repositories WHERE project_id = ?", id).Scan(&repository)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", time.Now().UnixNano(), id); err != nil {
		return result, err
	}
	result = model.ProjectInitResult{ProjectID: id, Identifier: identity.Path}
	if repository != "" {
		result.Identifier = repository
	}
	return result, tx.Commit()
}
