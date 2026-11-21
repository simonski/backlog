package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	Todo     = "todo"
	Doing    = "doing"
	Blocked  = "blocked"
	Done     = "done"
	Rejected = "rejected"
)

type PeriodActivity struct {
	Start     string
	ProjectID int64
	Status    string
	Count     int
}

type Invalid string

func (err Invalid) Error() string { return string(err) }

type TaskUpdate struct {
	ID          string  `json:"id"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Progress    *int    `json:"progress,omitempty"`
	Blocker     *string `json:"blocker,omitempty"`
}

type Report struct {
	Root     string       `json:"root"`
	Snapshot bool         `json:"snapshot"`
	Tasks    []TaskUpdate `json:"tasks"`
}

type TaskChanges struct {
	Root  string       `json:"root"`
	Tasks []TaskUpdate `json:"tasks"`
}

func (report Report) Validate() error {
	if report.Tasks == nil || (!report.Snapshot && len(report.Tasks) != 1) {
		return Invalid("a task report needs one task and a snapshot needs a task array")
	}
	return validateTasks(report.Root, report.Tasks)
}

func (changes TaskChanges) Validate() error {
	if len(changes.Tasks) == 0 {
		return Invalid("at least one task update is required")
	}
	if err := validateTasks(changes.Root, changes.Tasks); err != nil {
		return err
	}
	for _, task := range changes.Tasks {
		if task.Title == nil && task.Description == nil && task.Status == nil && task.Progress == nil && task.Blocker == nil {
			return Invalid(fmt.Sprintf("task %q: at least one field to update is required", task.ID))
		}
	}
	return nil
}

func ValidateRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return Invalid("project root must be an absolute, clean path")
	}
	return nil
}

func ValidateTaskIDs(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return Invalid("task ID is required")
		}
		if seen[id] {
			return Invalid(fmt.Sprintf("duplicate task ID %q", id))
		}
		seen[id] = true
	}
	return nil
}

func validateTasks(root string, tasks []TaskUpdate) error {
	if err := ValidateRoot(root); err != nil {
		return err
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
		if err := task.validate(); err != nil {
			return Invalid(fmt.Sprintf("task %q: %s", task.ID, err))
		}
	}
	return ValidateTaskIDs(ids)
}

func (task TaskUpdate) validate() error {
	if task.Title != nil && strings.TrimSpace(*task.Title) == "" {
		return Invalid("title cannot be blank")
	}
	if task.Status != nil {
		switch *task.Status {
		case Todo, Doing, Blocked, Done:
		default:
			return Invalid("status must be todo, doing, blocked, or done")
		}
	}
	if task.Progress != nil && (*task.Progress < 0 || *task.Progress > 100) {
		return Invalid("progress must be between 0 and 100")
	}
	return nil
}

type Project struct {
	ID            int64
	Root          string
	Archived      bool
	LatestReport  int64
	Todo          int
	Doing         int
	Blocked       int
	Done          int
	Omitted       int
	Readme        string
	Specification string
	ColorPreset   string
}

func (project Project) Name() string {
	return filepath.Base(project.Root)
}

type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Progress    *int   `json:"progress"`
	Blocker     string `json:"blocker"`
	Omitted     bool   `json:"omitted"`
	ActiveNanos *int64 `json:"-"`
	StartedAt   *int64 `json:"-"`
}

func (task Task) DisplayProgress() *int {
	if task.Status == Done {
		complete := 100
		return &complete
	}
	return task.Progress
}

type Result struct {
	ProjectID int64 `json:"project_id"`
	Reported  int   `json:"reported"`
}
