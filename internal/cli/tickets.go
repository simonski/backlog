package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
)

func ticketIDs(value string) []string {
	ids := strings.Split(value, ",")
	for i, id := range ids {
		ids[i] = strings.TrimSpace(id)
	}
	return ids
}

func listTickets(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("ls", diagnostics, " [-project N] [-id ID[,ID...]] [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var projectID string
	flags.Func("project", "list tickets for this project ID", func(value string) error {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return model.Invalid("project ID must be a positive number")
		}
		projectID = value
		return nil
	})
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "override the dashboard with a local host:port")
	var ids []string
	flags.Func("id", "comma-separated ticket IDs (default all tickets)", func(value string) error {
		ids = ticketIDs(value)
		return model.ValidateTaskIDs(ids)
	})
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	query := url.Values{"tickets": {"1"}, "id": ids}
	if projectID != "" {
		query.Set("project", projectID)
	} else {
		projectHints(ctx, query)
	}
	projects, err := fetchProjects(ctx, flags, address, query, diagnostics)
	if err != nil {
		return err
	}
	if *asJSON {
		type projectTicket struct {
			model.Task
			ProjectID int64  `json:"project_id"`
			Project   string `json:"project"`
		}
		tasks := make([]projectTicket, 0)
		for _, project := range projects {
			for _, task := range project.Tickets {
				tasks = append(tasks, projectTicket{Task: task, ProjectID: project.ID, Project: project.Name})
			}
		}
		return writeJSON(out, tasks)
	}
	if len(projects) == 0 {
		_, err := fmt.Fprintln(out, "No projects tracked.")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for i, project := range projects {
		if i != 0 {
			fmt.Fprintln(table)
		}
		fmt.Fprintf(table, "%s [%d]\n", ticketCell(projectLabel(project)), project.ID)
		if len(project.Tickets) == 0 {
			fmt.Fprintln(table, "No tickets.")
			continue
		}
		fmt.Fprintln(table, "ID\tSTATUS\tPROGRESS\tTITLE")
		for _, task := range project.Tickets {
			status := task.Status
			if task.Omitted {
				status = model.Rejected
			}
			progress := "-"
			if value := task.DisplayProgress(); value != nil {
				progress = strconv.Itoa(*value) + "%"
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", ticketCell(task.ID), status, progress, ticketCell(task.Title))
		}
	}
	return table.Flush()
}

func ticketCell(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}

func updateTickets(ctx context.Context, command string, args []string, out, diagnostics io.Writer) error {
	var address string
	var asJSON bool
	update, err := parseTask(command, args, &address, &asJSON, diagnostics)
	if err != nil {
		return err
	}
	root, err := projectRoot(ctx)
	if err != nil {
		return err
	}
	changes := model.TaskChanges{Root: root}
	for _, id := range ticketIDs(update.ID) {
		update.ID = id
		changes.Tasks = append(changes.Tasks, update)
	}
	if err := changes.Validate(); err != nil {
		return err
	}
	var result model.Result
	if err := requestServer(ctx, address, http.MethodPatch, "/api/tasks", changes, &result, diagnostics); err != nil {
		return err
	}
	if asJSON {
		return writeJSON(out, result)
	}
	_, err = fmt.Fprintf(out, "Project ID:   %d\nTasks updated: %d\n", result.ProjectID, result.Reported)
	return err
}
