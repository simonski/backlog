package storage

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"harness.engineering/internal/model"
)

func (store *Store) ListProjects(ctx context.Context, selection model.ProjectContext, tickets, local bool, ids []string) ([]model.ProjectListing, error) {
	if err := model.ValidateTaskIDs(ids); err != nil {
		return nil, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	projects, err := store.listProjects(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	if !local {
		projects = slices.DeleteFunc(projects, func(project model.Project) bool { return project.Archived })
	}
	if selection.ID != 0 {
		index := slices.IndexFunc(projects, func(project model.Project) bool { return project.ID == selection.ID })
		if index < 0 {
			return nil, model.Invalid(fmt.Sprintf("project %d not found", selection.ID))
		}
		projects = projects[index : index+1]
	} else {
		inferred, err := store.inferProject(ctx, tx, projects, selection)
		if err != nil {
			return nil, err
		}
		if inferred != 0 {
			projects = slices.DeleteFunc(projects, func(project model.Project) bool { return project.ID != inferred })
		}
	}

	found := make(map[string]bool, len(ids))
	result := make([]model.ProjectListing, 0, len(projects))
	for _, project := range projects {
		entry := model.ProjectListing{
			ID: project.ID, Name: project.Name(), Archived: project.Archived, UpdatedAt: project.LatestReport, Repository: project.Repository,
			Todo: project.Todo, Doing: project.Doing, Blocked: project.Blocked, Done: project.Done, Rejected: project.Omitted,
		}
		if local {
			entry.Root = project.PortableRoot
		}
		if tickets {
			entry.Tickets, err = listTasks(ctx, tx, project.ID, local)
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				slices.SortStableFunc(entry.Tickets, compareTicketActivity)
			} else {
				byID := make(map[string]model.Task, len(entry.Tickets))
				for _, task := range entry.Tickets {
					byID[task.ID] = task
				}
				entry.Tickets = entry.Tickets[:0]
				for _, id := range ids {
					if task, ok := byID[id]; ok {
						entry.Tickets = append(entry.Tickets, task)
						found[id] = true
					}
				}
				if len(entry.Tickets) == 0 {
					continue
				}
			}
		}
		result = append(result, entry)
	}
	for _, id := range ids {
		if !found[id] {
			return nil, model.Invalid(fmt.Sprintf("task %q not found", id))
		}
	}
	return result, nil
}

func compareTicketActivity(a, b model.Task) int {
	aDoing := a.Status == model.Doing && !a.Omitted
	bDoing := b.Status == model.Doing && !b.Omitted
	if aDoing != bDoing {
		if aDoing {
			return -1
		}
		return 1
	}
	return cmp.Compare(b.UpdatedAt, a.UpdatedAt)
}

func (store *Store) inferProject(ctx context.Context, tx *sql.Tx, projects []model.Project, selection model.ProjectContext) (int64, error) {
	for _, project := range projects {
		if project.Repository != "" && strings.EqualFold(project.Repository, selection.Repository) {
			return project.ID, nil
		}
	}
	paths := []string{selection.Directory, selection.GitRoot, selection.CommonRoot}
	for _, name := range paths {
		root, err := model.ResolveProjectPath(name, store.home)
		if err != nil {
			continue
		}
		id, err := projectForRoot(ctx, tx, root, false)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if slices.ContainsFunc(projects, func(project model.Project) bool { return project.ID == id }) {
			return id, nil
		}
	}

	var ancestor model.Project
	if directory, err := model.ResolveProjectPath(selection.Directory, store.home); err == nil {
		for _, project := range projects {
			relative, err := filepath.Rel(project.Root, directory)
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
				len(project.Root) > len(ancestor.Root) {
				ancestor = project
			}
		}
	}
	if ancestor.ID != 0 {
		return ancestor.ID, nil
	}

	for _, name := range []string{path.Base(selection.Repository), path.Base(selection.GitRoot), path.Base(selection.Directory)} {
		if name == "." || name == "/" || name == "~" {
			continue
		}
		var match int64
		for _, project := range projects {
			if strings.EqualFold(project.Name(), name) {
				if match != 0 {
					return 0, nil
				}
				match = project.ID
			}
		}
		if match != 0 {
			return match, nil
		}
	}
	return 0, nil
}
