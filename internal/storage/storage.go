package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"harness.engineering/internal/model"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY,
	root TEXT NOT NULL UNIQUE,
	archived INTEGER NOT NULL DEFAULT 0,
	latest_report INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tasks (
	project_id INTEGER NOT NULL REFERENCES projects(id),
	id TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'doing', 'blocked', 'done')),
	progress INTEGER CHECK (progress BETWEEN 0 AND 100),
	blocker TEXT NOT NULL DEFAULT '',
	omitted INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (project_id, id)
);

CREATE TABLE IF NOT EXISTS daily_task_activity (
	day TEXT NOT NULL,
	project_id INTEGER NOT NULL,
	task_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('todo', 'doing', 'blocked', 'done', 'rejected')),
	PRIMARY KEY (day, project_id, task_id),
	FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id)
);
`

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := url.Values{"_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	// Serialize transactions on this server so every report sees the prior commit.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (store *Store) Close() error { return store.db.Close() }

func (store *Store) Apply(ctx context.Context, report model.Report) (model.Result, error) {
	if err := report.Validate(); err != nil {
		return model.Result{}, err
	}
	return store.apply(ctx, report.Root, report.Tasks, report.Snapshot, true)
}

func (store *Store) UpdateTasks(ctx context.Context, changes model.TaskChanges) (model.Result, error) {
	if err := changes.Validate(); err != nil {
		return model.Result{}, err
	}
	return store.apply(ctx, changes.Root, changes.Tasks, false, false)
}

func (store *Store) apply(ctx context.Context, root string, tasks []model.TaskUpdate, snapshot, allowCreate bool) (model.Result, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Result{}, err
	}
	defer tx.Rollback()
	if allowCreate {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO projects(root) VALUES (?) ON CONFLICT(root) DO NOTHING
		`, root); err != nil {
			return model.Result{}, err
		}
	}
	var projectID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE root = ?", root).Scan(&projectID); errors.Is(err, sql.ErrNoRows) {
		return model.Result{}, model.Invalid("no tasks found for this project")
	} else if err != nil {
		return model.Result{}, err
	}

	now := time.Now()
	day := now.Format(time.DateOnly)
	if snapshot {
		// Included tasks overwrite their provisional rejection below.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_task_activity (day, project_id, task_id, status)
			SELECT ?, project_id, id, 'rejected' FROM tasks
			WHERE project_id = ? AND status <> 'done' AND omitted = 0
			ON CONFLICT (day, project_id, task_id) DO UPDATE SET status = excluded.status
		`, day, projectID); err != nil {
			return model.Result{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET omitted = 1 WHERE project_id = ? AND status <> 'done' AND omitted = 0
		`, projectID); err != nil {
			return model.Result{}, err
		}
	}
	for _, task := range tasks {
		if err := applyTask(ctx, tx, projectID, task, allowCreate); err != nil {
			return model.Result{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_task_activity (day, project_id, task_id, status)
			SELECT ?, project_id, id, status FROM tasks WHERE project_id = ? AND id = ?
			ON CONFLICT (day, project_id, task_id) DO UPDATE SET status = excluded.status
		`, day, projectID, task.ID); err != nil {
			return model.Result{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET latest_report = ? WHERE id = ?", now.UnixNano(), projectID); err != nil {
		return model.Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Result{}, err
	}
	return model.Result{ProjectID: projectID, Reported: len(tasks)}, nil
}

func applyTask(ctx context.Context, tx *sql.Tx, projectID int64, task model.TaskUpdate, allowCreate bool) error {
	updated, err := tx.ExecContext(ctx, `
		UPDATE tasks SET title = COALESCE(?, title), description = COALESCE(?, description),
			status = COALESCE(?, status), progress = COALESCE(?, progress),
			blocker = COALESCE(?, blocker), omitted = 0
		WHERE project_id = ? AND id = ?
	`, task.Title, task.Description, task.Status, task.Progress, task.Blocker, projectID, task.ID)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil || count > 0 {
		return err
	}
	if !allowCreate {
		return model.Invalid(fmt.Sprintf("task %q not found", task.ID))
	}
	if task.Title == nil {
		return model.Invalid(fmt.Sprintf("new task %q requires a title", task.ID))
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (project_id, id, title, description, status, progress, blocker)
		VALUES (?, ?, ?, COALESCE(?, ''), COALESCE(?, 'todo'), ?, COALESCE(?, ''))
	`, projectID, task.ID, task.Title, task.Description, task.Status, task.Progress, task.Blocker)
	return err
}

func (store *Store) SetArchived(ctx context.Context, projectID int64, archived bool) error {
	result, err := store.db.ExecContext(ctx, "UPDATE projects SET archived = ? WHERE id = ?", archived, projectID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

// Dashboard reads the navigation and selected project from one consistent view.
func (store *Store) Dashboard(ctx context.Context, selectedID int64) ([]model.Project, []model.Task, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	projects, err := listProjects(ctx, tx)
	if err != nil || selectedID == 0 {
		return projects, nil, err
	}
	for _, project := range projects {
		if project.ID == selectedID {
			tasks, err := listTasks(ctx, tx, selectedID)
			return projects, tasks, err
		}
	}
	return nil, nil, sql.ErrNoRows
}

func listProjects(ctx context.Context, tx *sql.Tx) ([]model.Project, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.root, p.archived, p.latest_report,
			COUNT(CASE WHEN t.status = 'todo' AND t.omitted = 0 THEN 1 END),
			COUNT(CASE WHEN t.status = 'doing' AND t.omitted = 0 THEN 1 END),
			COUNT(CASE WHEN t.status = 'blocked' AND t.omitted = 0 THEN 1 END),
			COUNT(CASE WHEN t.status = 'done' THEN 1 END),
			COUNT(CASE WHEN t.omitted = 1 THEN 1 END)
		FROM projects p LEFT JOIN tasks t ON t.project_id = p.id
		GROUP BY p.id
		ORDER BY p.latest_report DESC, p.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var project model.Project
		if err := rows.Scan(&project.ID, &project.Root, &project.Archived, &project.LatestReport,
			&project.Todo, &project.Doing, &project.Blocked, &project.Done, &project.Omitted); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func listTasks(ctx context.Context, tx *sql.Tx, projectID int64) ([]model.Task, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, description, status, progress, blocker, omitted
		FROM tasks WHERE project_id = ? ORDER BY title COLLATE NOCASE, id
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		var task model.Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.Progress, &task.Blocker, &task.Omitted); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (store *Store) ListTasks(ctx context.Context, root string, ids []string) ([]model.Task, error) {
	if err := model.ValidateRoot(root); err != nil {
		return nil, err
	}
	if err := model.ValidateTaskIDs(ids); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var projectID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE root = ?", root).Scan(&projectID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var tasks []model.Task
	if err == nil {
		tasks, err = listTasks(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		if tasks == nil {
			tasks = []model.Task{}
		}
		return tasks, nil
	}
	byID := make(map[string]model.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	selected := make([]model.Task, 0, len(ids))
	for _, id := range ids {
		task, ok := byID[id]
		if !ok {
			return nil, model.Invalid(fmt.Sprintf("task %q not found", id))
		}
		selected = append(selected, task)
	}
	return selected, nil
}

func IsMissing(err error) bool { return errors.Is(err, sql.ErrNoRows) }
