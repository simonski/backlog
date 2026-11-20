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
	flags := newFlags("ls", diagnostics, " [-id ID[,ID...]] [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "local server host:port")
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
	root, err := projectRoot(ctx)
	if err != nil {
		return err
	}
	query := url.Values{"root": {root}, "id": ids}
	var tasks []model.Task
	if err := requestServer(ctx, address, http.MethodGet, "/api/tasks?"+query.Encode(), nil, &tasks, diagnostics); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, tasks)
	}
	if len(tasks) == 0 {
		_, err := fmt.Fprintln(out, "No tickets for this project.")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTATUS\tPROGRESS\tTITLE")
	for _, task := range tasks {
		status := task.Status
		if task.Omitted {
			status = "rejected"
		}
		progress := "-"
		if value := task.DisplayProgress(); value != nil {
			progress = strconv.Itoa(*value) + "%"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", ticketCell(task.ID), status, progress, ticketCell(task.Title))
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
