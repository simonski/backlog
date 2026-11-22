package storage

import (
	"context"
	"database/sql"
	"time"

	"harness.engineering/internal/model"
)

func (store *Store) SaveDocument(ctx context.Context, document model.DocumentUpdate) (int64, error) {
	if err := document.Validate(); err != nil {
		return 0, err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	projectID, err := projectForRoot(ctx, tx, document.Root, true)
	if err != nil {
		return 0, err
	}
	uploadedAt := time.Now().UnixNano()
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", uploadedAt, projectID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_documents (project_id, name, markdown) VALUES (?, ?, ?)
		ON CONFLICT (project_id, name) DO UPDATE SET markdown = excluded.markdown
	`, projectID, document.Name, document.Markdown); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_document_activity (project_id, uploaded_at) VALUES (?, ?)
		ON CONFLICT (project_id) DO UPDATE SET uploaded_at = excluded.uploaded_at
	`, projectID, uploadedAt); err != nil {
		return 0, err
	}
	return projectID, tx.Commit()
}

func listDocumentNames(ctx context.Context, tx *sql.Tx, projectID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT name FROM project_documents WHERE project_id = ? ORDER BY name COLLATE NOCASE, name
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (store *Store) Document(ctx context.Context, projectID int64, name string) (model.DocumentUpdate, error) {
	document := model.DocumentUpdate{Name: name}
	err := store.db.QueryRowContext(ctx, `
		SELECT p.root, d.markdown FROM project_documents d JOIN projects p ON p.id = d.project_id
		WHERE d.project_id = ? AND d.name = ? AND p.archived = 0
	`, projectID, name).Scan(&document.Root, &document.Markdown)
	return document, err
}

func (store *Store) ClearDocuments(ctx context.Context, root string) (model.DocumentClearResult, error) {
	var receipt model.DocumentClearResult
	if err := model.ValidateRoot(root); err != nil {
		return receipt, err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	receipt.ProjectID, err = projectForRoot(ctx, tx, root, false)
	if err == sql.ErrNoRows {
		return receipt, model.Invalid("no project found for this directory")
	} else if err != nil {
		return receipt, err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM project_documents WHERE project_id = ?", receipt.ProjectID)
	if err != nil {
		return receipt, err
	}
	receipt.Removed, err = result.RowsAffected()
	if err != nil {
		return receipt, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_document_activity WHERE project_id = ?", receipt.ProjectID); err != nil {
		return receipt, err
	}
	if receipt.Removed > 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", time.Now().UnixNano(), receipt.ProjectID); err != nil {
			return receipt, err
		}
	}
	return receipt, tx.Commit()
}
