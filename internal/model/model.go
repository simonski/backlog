package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	Todo    = "todo"
	Doing   = "doing"
	Blocked = "blocked"
	Done    = "done"
)

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

func (report Report) Validate() error {
	if !filepath.IsAbs(report.Root) || filepath.Clean(report.Root) != report.Root {
		return Invalid("project root must be an absolute, clean path")
	}
	if report.Tasks == nil || (!report.Snapshot && len(report.Tasks) != 1) {
		return Invalid("a task report needs one task and a snapshot needs a task array")
	}
	seen := make(map[string]bool, len(report.Tasks))
	for _, task := range report.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return Invalid("task ID is required")
		}
		if seen[task.ID] {
			return Invalid(fmt.Sprintf("duplicate task ID %q", task.ID))
		}
		seen[task.ID] = true
		if err := task.validate(); err != nil {
			return Invalid(fmt.Sprintf("task %q: %s", task.ID, err))
		}
	}
	return nil
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
	ID           int64
	Root         string
	Archived     bool
	LatestReport int64
	Todo         int
	Doing        int
	Blocked      int
	Done         int
	Omitted      int
}

func (project Project) Name() string {
	return filepath.Base(project.Root)
}

type Task struct {
	ID          string
	Title       string
	Description string
	Status      string
	Progress    *int
	Blocker     string
	Omitted     bool
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
