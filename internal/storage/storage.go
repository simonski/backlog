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

CREATE TABLE IF NOT EXISTS project_repositories (
	project_id INTEGER PRIMARY KEY REFERENCES projects(id),
	repository TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE IF NOT EXISTS project_locations (
	root TEXT PRIMARY KEY,
	project_id INTEGER NOT NULL REFERENCES projects(id)
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

CREATE TABLE IF NOT EXISTS task_timing (
	project_id INTEGER NOT NULL,
	task_id TEXT NOT NULL,
	active_nanos INTEGER NOT NULL DEFAULT 0,
	started_at INTEGER,
	PRIMARY KEY (project_id, task_id),
	FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id)
);

CREATE TABLE IF NOT EXISTS task_updates (
	project_id INTEGER NOT NULL,
	task_id TEXT NOT NULL,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY (project_id, task_id),
	FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id)
);

CREATE TABLE IF NOT EXISTS archived_tasks (
	project_id INTEGER NOT NULL,
	task_id TEXT NOT NULL,
	PRIMARY KEY (project_id, task_id),
	FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id)
);

CREATE TABLE IF NOT EXISTS project_documents (
	project_id INTEGER NOT NULL REFERENCES projects(id),
	name TEXT NOT NULL,
	markdown TEXT NOT NULL,
	PRIMARY KEY (project_id, name)
);

CREATE TABLE IF NOT EXISTS project_document_activity (
	project_id INTEGER PRIMARY KEY REFERENCES projects(id),
	uploaded_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS task_completions (
	project_id INTEGER NOT NULL,
	task_id TEXT NOT NULL,
	PRIMARY KEY (project_id, task_id),
	FOREIGN KEY (project_id, task_id) REFERENCES tasks(project_id, id)
);

CREATE TABLE IF NOT EXISTS project_colors (
	project_id INTEGER PRIMARY KEY REFERENCES projects(id),
	preset TEXT NOT NULL UNIQUE
);
`

type Store struct {
	db   *sql.DB
	home string
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
	if _, err := db.ExecContext(ctx, `
		INSERT INTO task_completions (project_id, task_id)
		SELECT project_id, id FROM tasks WHERE status = 'done'
		UNION SELECT project_id, task_id FROM daily_task_activity WHERE status = 'done'
		ON CONFLICT (project_id, task_id) DO NOTHING
	`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO task_timing (project_id, task_id, started_at)
	SELECT t.project_id, t.id, ? FROM tasks t WHERE t.status = 'doing' AND t.omitted = 0
	AND NOT EXISTS (SELECT 1 FROM archived_tasks ar WHERE ar.project_id = t.project_id AND ar.task_id = t.id)
		ON CONFLICT (project_id, task_id) DO NOTHING
	`, time.Now().UnixNano()); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO task_updates (project_id, task_id, updated_at)
		SELECT a.project_id, a.task_id, unixepoch(MAX(a.day), 'utc') * 1000000000
		FROM daily_task_activity a LEFT JOIN task_updates u ON u.project_id = a.project_id AND u.task_id = a.task_id
		WHERE u.task_id IS NULL GROUP BY a.project_id, a.task_id
		ON CONFLICT (project_id, task_id) DO NOTHING
	`); err != nil {
		db.Close()
		return nil, err
	}
	home, _ := os.UserHomeDir()
	if canonical, err := filepath.EvalSymlinks(home); home != "" && err == nil {
		home = canonical
	}
	return &Store{db: db, home: home}, nil
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
	projectID, err := projectForRoot(ctx, tx, root, allowCreate)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Result{}, model.Invalid("no tasks found for this project")
	} else if err != nil {
		return model.Result{}, err
	}

	now := time.Now()
	day := now.Format(time.DateOnly)
	if snapshot {
		// Included tasks overwrite their provisional rejection below.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_updates (project_id, task_id, updated_at)
			SELECT project_id, id, ? FROM tasks WHERE project_id = ? AND status <> 'done' AND omitted = 0
			ON CONFLICT (project_id, task_id) DO UPDATE SET updated_at = excluded.updated_at
		`, now.UnixNano(), projectID); err != nil {
			return model.Result{}, err
		}
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
			INSERT INTO task_updates (project_id, task_id, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (project_id, task_id) DO UPDATE SET updated_at = excluded.updated_at
		`, projectID, task.ID, now.UnixNano()); err != nil {
			return model.Result{}, err
		}
		if task.Status != nil && *task.Status == model.Done {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO task_completions (project_id, task_id) VALUES (?, ?)
				ON CONFLICT (project_id, task_id) DO NOTHING
			`, projectID, task.ID); err != nil {
				return model.Result{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_task_activity (day, project_id, task_id, status)
			SELECT ?, project_id, id, status FROM tasks WHERE project_id = ? AND id = ?
			ON CONFLICT (day, project_id, task_id) DO UPDATE SET status = excluded.status
		`, day, projectID, task.ID); err != nil {
			return model.Result{}, err
		}
	}
	if err := syncTaskTiming(ctx, tx, projectID, now.UnixNano()); err != nil {
		return model.Result{}, err
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
func (store *Store) Dashboard(ctx context.Context, selectedID int64) ([]model.Project, []model.Task, []string, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback()

	projects, err := store.listProjects(ctx, tx, 0)
	if err != nil || selectedID == 0 {
		return projects, nil, nil, err
	}
	for _, project := range projects {
		if project.ID == selectedID && !project.Archived {
			tasks, err := listTasks(ctx, tx, selectedID, false)
			if err != nil {
				return nil, nil, nil, err
			}
			documents, err := listDocumentNames(ctx, tx, selectedID)
			return projects, tasks, documents, err
		}
	}
	return nil, nil, nil, sql.ErrNoRows
}

func (store *Store) listProjects(ctx context.Context, tx *sql.Tx, promoteUploadsAfter int64) ([]model.Project, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.root, p.archived, p.latest_report,
			COUNT(CASE WHEN t.status = 'todo' AND t.omitted = 0 THEN 1 END),
			COUNT(CASE WHEN t.status = 'doing' AND t.omitted = 0 THEN 1 END) AS doing_count,
			COUNT(CASE WHEN t.status = 'blocked' AND t.omitted = 0 THEN 1 END),
			(SELECT COUNT(*) FROM task_completions c WHERE c.project_id = p.id),
			COUNT(CASE WHEN t.omitted = 1 THEN 1 END),
			COALESCE((SELECT name FROM project_documents WHERE project_id = p.id
				AND lower(name) IN ('readme', 'readme.md', 'readme.markdown')
				ORDER BY CASE lower(name) WHEN 'readme.md' THEN 0 ELSE 1 END, name LIMIT 1), ''),
			COALESCE((SELECT name FROM project_documents WHERE project_id = p.id
				AND lower(name) IN ('spec', 'spec.md', 'spec.markdown', 'specification', 'specification.md', 'specification.markdown')
				ORDER BY CASE lower(name) WHEN 'specification.md' THEN 0 WHEN 'spec.md' THEN 1 ELSE 2 END, name LIMIT 1), ''),
			COALESCE(color.preset, ''), COALESCE(repo.repository, '')
		FROM projects p LEFT JOIN tasks t ON t.project_id = p.id AND NOT EXISTS (
			SELECT 1 FROM archived_tasks ar WHERE ar.project_id = t.project_id AND ar.task_id = t.id
		)
		LEFT JOIN project_document_activity d ON d.project_id = p.id
		LEFT JOIN project_colors color ON color.project_id = p.id
		LEFT JOIN project_repositories repo ON repo.project_id = p.id
		GROUP BY p.id
		ORDER BY CASE WHEN ?1 <> 0 AND d.uploaded_at > ?1 THEN d.uploaded_at ELSE 0 END DESC,
			(doing_count > 0) DESC, p.latest_report DESC, p.id DESC
	`, promoteUploadsAfter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var project model.Project
		if err := rows.Scan(&project.ID, &project.Root, &project.Archived, &project.LatestReport,
			&project.Todo, &project.Doing, &project.Blocked, &project.Done, &project.Omitted,
			&project.Readme, &project.Specification, &project.ColorPreset, &project.Repository); err != nil {
			return nil, err
		}
		project.PortableRoot = model.PortablePath(project.Root, store.home)
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func listTasks(ctx context.Context, tx *sql.Tx, projectID int64, includeArchived bool) ([]model.Task, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id, t.title, t.description, t.status, t.progress, t.blocker, t.omitted, tm.active_nanos, tm.started_at,
			COALESCE(u.updated_at, 0)
		FROM tasks t LEFT JOIN task_timing tm ON tm.project_id = t.project_id AND tm.task_id = t.id
		LEFT JOIN task_updates u ON u.project_id = t.project_id AND u.task_id = t.id
		WHERE t.project_id = ? AND (? OR NOT EXISTS (
			SELECT 1 FROM archived_tasks ar WHERE ar.project_id = t.project_id AND ar.task_id = t.id
		)) ORDER BY t.title COLLATE NOCASE, t.id
	`, projectID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		var task model.Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.Progress, &task.Blocker,
			&task.Omitted, &task.ActiveNanos, &task.StartedAt, &task.UpdatedAt); err != nil {
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
	projectID, err := projectForRoot(ctx, tx, root, false)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var tasks []model.Task
	if err == nil {
		tasks, err = listTasks(ctx, tx, projectID, true)
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
