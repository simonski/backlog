package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"harness.engineering/internal/local"
	"harness.engineering/internal/menubar"
	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

const usage = `Usage: backlog <command> [options]

Commands:
  setup          Configure a client or review the local server setup
  serve          Run the dashboard and reporting server
  status         Check Backlog setup, processes, and server health
  ps             Alias for status
  restart        Print the command to restart or start the launchd service
  upgrade        Bring the local launchd server up to this build's version
%s  task           Report a single task update
  snapshot       Report all currently known project tasks as a JSON array
  ls             List projects and their tickets
  project        Show project commands
  update         Edit one or more existing tickets
  block          Mark tickets blocked
  todo           Mark tickets to do
  doing          Mark tickets in progress
  done           Mark tickets complete
  doc            Publish, replace, or clear uploaded project documents
  install-skill  Install the embedded harness reporting skill
  version        Show the binary's application and protocol versions

Run backlog <command> -help for command options.
`

type App struct {
	ReportingSkill []byte
}

func writeUsage(w io.Writer) {
	menu := ""
	if menubar.Available {
		menu = "  menu           Run the Backlog client in the macOS menu bar\n"
	}
	fmt.Fprintf(w, usage, menu)
}

func (app App) Run(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) int {
	if len(args) == 0 {
		writeUsage(diagnostics)
		return 2
	}
	var err error
	switch args[0] {
	case "help", "-help", "-h":
		writeUsage(out)
	case "setup":
		err = setup(ctx, args[1:], input, out, diagnostics)
	case "serve":
		err = serve(ctx, args[1:], out, diagnostics)
	case "status", "ps":
		err = showStatus(ctx, args[0], args[1:], out, diagnostics)
	case "restart":
		err = showRestart(ctx, args[1:], out, diagnostics)
	case "upgrade":
		err = upgradeServer(ctx, args[1:], out, diagnostics)
	case "menu":
		err = runMenu(ctx, args[1:], diagnostics)
	case "task", "snapshot":
		err = sendReport(ctx, args[0], args[1:], input, out, diagnostics)
	case "ls":
		err = listTickets(ctx, args[1:], out, diagnostics)
	case "project":
		err = projectCommand(ctx, args[1:], out, diagnostics)
	case "update", "block", "todo", "doing", "done":
		err = updateTickets(ctx, args[0], args[1:], out, diagnostics)
	case "doc":
		err = uploadDocument(ctx, args[1:], input, out, diagnostics)
	case "install-skill":
		err = installSkill(args[1:], app.ReportingSkill, out, diagnostics)
	case "version":
		err = showVersion(args[1:], out, diagnostics)
	default:
		err = fmt.Errorf("unknown command %q (run backlog -help)", args[0])
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(diagnostics, "backlog: %v\n", err)
		return 1
	}
	return 0
}

func showVersion(args []string, out, diagnostics io.Writer) error {
	flags := newFlags("version", diagnostics, " [-json]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, struct {
			Client   string `json:"client_version"`
			Server   string `json:"server_version"`
			Protocol int    `json:"protocol_version"`
		}{version.Client, version.Server, version.Protocol})
	}
	_, err := fmt.Fprintf(out, "CLI version:      %s\nServer version:   %s\nProtocol version: %d\n", version.Client, version.Server, version.Protocol)
	return err
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func newFlags(name string, diagnostics io.Writer, example string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	flags.Usage = func() {
		fmt.Fprintf(diagnostics, "Usage: backlog %s%s\n", name, example)
		flags.PrintDefaults()
	}
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	return nil
}

func addressFlags(flags *flag.FlagSet, address *string, fallback, description string) func() error {
	flags.StringVar(address, "addr", local.Address(fallback), description)
	port := ""
	flags.Func("p", "override the port", func(value string) error {
		number, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			return errors.New("port must be a number from 0 to 65535")
		}
		port = strconv.FormatUint(number, 10)
		return nil
	})
	return func() error {
		if port == "" {
			return nil
		}
		host, _, err := net.SplitHostPort(*address)
		if err != nil {
			return fmt.Errorf("invalid address: %w", err)
		}
		*address = net.JoinHostPort(host, port)
		return nil
	}
}

func sendReport(ctx context.Context, command string, args []string, input io.Reader, out, diagnostics io.Writer) error {
	var report model.Report
	var err error
	var address string
	var asJSON bool
	if command == "task" {
		var update model.TaskUpdate
		update, err = parseTask("task", args, &address, &asJSON, diagnostics)
		report.Tasks = []model.TaskUpdate{update}
	} else {
		report.Snapshot = true
		report.Tasks, err = parseSnapshot(args, &address, &asJSON, input, diagnostics)
	}
	if err != nil {
		return err
	}
	report.Root, err = projectRoot(ctx)
	if err != nil {
		return err
	}
	if err := report.Validate(); err != nil {
		return err
	}
	var result model.Result
	if err := requestServer(ctx, address, http.MethodPost, "/api/reports", report, &result, diagnostics); err != nil {
		return err
	}
	if asJSON {
		return writeJSON(out, result)
	}
	_, err = fmt.Fprintf(out, "Project ID:     %d\nTasks reported: %d\n", result.ProjectID, result.Reported)
	return err
}

func parseTask(command string, args []string, address *string, asJSON *bool, diagnostics io.Writer) (model.TaskUpdate, error) {
	example, idHelp := " -id ID[,ID...] [options]", "comma-separated ticket IDs"
	if command == "task" {
		example, idHelp = " -id ID [options]", "stable task ID"
	}
	flags := newFlags(command, diagnostics, example)
	flags.BoolVar(asJSON, "json", false, "print indented JSON")
	resolveAddress := addressFlags(flags, address, local.DefaultAddress, "local server host:port")
	var task model.TaskUpdate
	flags.StringVar(&task.ID, "id", "", idHelp)
	var title, description, status, blocker string
	var progress int
	if command == "task" || command == "update" {
		titleHelp := "task title"
		if command == "task" {
			titleHelp += " (required for a new task)"
		}
		flags.StringVar(&title, "title", "", titleHelp)
		flags.StringVar(&description, "description", "", "task description")
		flags.StringVar(&status, "status", "", "todo, doing, blocked, or done")
		flags.IntVar(&progress, "progress", 0, "completion percentage, 0 to 100")
	} else {
		status = command
		if command == "block" {
			status = model.Blocked
		}
		task.Status = &status
	}
	if command == "task" || command == "update" || command == "block" {
		flags.StringVar(&blocker, "blocker", "", "blocker note")
	}
	if err := parseFlags(flags, args); err != nil {
		return task, err
	}
	flags.Visit(func(field *flag.Flag) {
		switch field.Name {
		case "title":
			task.Title = &title
		case "description":
			task.Description = &description
		case "status":
			task.Status = &status
		case "progress":
			task.Progress = &progress
		case "blocker":
			task.Blocker = &blocker
		}
	})
	return task, resolveAddress()
}

func parseSnapshot(args []string, address *string, asJSON *bool, input io.Reader, diagnostics io.Writer) ([]model.TaskUpdate, error) {
	flags := newFlags("snapshot", diagnostics, " [options]")
	flags.BoolVar(asJSON, "json", false, "print indented JSON")
	resolveAddress := addressFlags(flags, address, local.DefaultAddress, "local server host:port")
	path := flags.String("file", "-", "JSON file, or - for standard input")
	if err := parseFlags(flags, args); err != nil {
		return nil, err
	}
	if err := resolveAddress(); err != nil {
		return nil, err
	}
	if *path != "-" {
		file, err := os.Open(*path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		input = file
	}
	payload, err := io.ReadAll(io.LimitReader(input, wire.MaxReportBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > wire.MaxReportBytes {
		return nil, errors.New("snapshot exceeds the 1 MiB report limit")
	}
	var tasks []model.TaskUpdate
	if err := wire.ReadJSON(bytes.NewReader(payload), &tasks); err != nil {
		return nil, fmt.Errorf("invalid snapshot: %w", err)
	}
	return tasks, nil
}

func projectRoot(ctx context.Context) (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "git", "-C", directory, "rev-parse", "--show-toplevel")
	command.Env = append(os.Environ(), "LC_ALL=C")
	output, err := command.Output()
	if err == nil {
		directory = strings.TrimSuffix(string(output), "\n")
	} else {
		var exit *exec.ExitError
		if !errors.Is(err, exec.ErrNotFound) &&
			!(errors.As(err, &exit) && strings.Contains(string(exit.Stderr), "not a git repository")) {
			return "", fmt.Errorf("resolve Git root: %w", err)
		}
	}
	return filepath.EvalSymlinks(directory)
}
