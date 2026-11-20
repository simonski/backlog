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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

const usage = `Usage: backlog <command> [options]

Commands:
  setup          Set up the background service and harness reporting skill
  serve          Run the dashboard and reporting server
  task           Report a single task update
  snapshot       Report all currently known project tasks as a JSON array
  install-skill  Install the embedded harness reporting skill
  version        Print the binary's application and protocol versions as JSON

Run backlog <command> -help for command options.
`

type App struct {
	ReportingSkill []byte
}

func (app App) Run(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(diagnostics, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "help", "-help", "-h":
		fmt.Fprint(out, usage)
	case "setup":
		err = setup(ctx, args[1:], app.ReportingSkill, input, out, diagnostics)
	case "serve":
		err = serve(ctx, args[1:], diagnostics)
	case "task", "snapshot":
		err = sendReport(ctx, args[0], args[1:], input, out, diagnostics)
	case "install-skill":
		err = installSkill(args[1:], app.ReportingSkill, out, diagnostics)
	case "version":
		flags := newFlags("version", diagnostics, "")
		err = parseFlags(flags, args[1:])
		if err == nil {
			err = json.NewEncoder(out).Encode(struct {
				Client   string `json:"client_version"`
				Server   string `json:"server_version"`
				Protocol int    `json:"protocol_version"`
			}{version.Client, version.Server, version.Protocol})
		}
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
	if command == "task" {
		var update model.TaskUpdate
		update, err = parseTask(args, &address, diagnostics)
		report.Tasks = []model.TaskUpdate{update}
	} else {
		report.Snapshot = true
		report.Tasks, err = parseSnapshot(args, &address, input, diagnostics)
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
	result, err := publish(ctx, address, report, diagnostics)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}

func parseTask(args []string, address *string, diagnostics io.Writer) (model.TaskUpdate, error) {
	flags := newFlags("task", diagnostics, " -id ID [options]")
	resolveAddress := addressFlags(flags, address, local.DefaultAddress, "local server host:port")
	var task model.TaskUpdate
	flags.StringVar(&task.ID, "id", "", "stable task ID")
	title := flags.String("title", "", "task title (required for a new task)")
	description := flags.String("description", "", "task description")
	status := flags.String("status", "", "todo, doing, blocked, or done")
	progress := flags.Int("progress", 0, "completion percentage, 0 to 100")
	blocker := flags.String("blocker", "", "blocker note")
	if err := parseFlags(flags, args); err != nil {
		return task, err
	}
	flags.Visit(func(field *flag.Flag) {
		switch field.Name {
		case "title":
			task.Title = title
		case "description":
			task.Description = description
		case "status":
			task.Status = status
		case "progress":
			task.Progress = progress
		case "blocker":
			task.Blocker = blocker
		}
	})
	return task, resolveAddress()
}

func parseSnapshot(args []string, address *string, input io.Reader, diagnostics io.Writer) ([]model.TaskUpdate, error) {
	flags := newFlags("snapshot", diagnostics, " [-file PATH]")
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
