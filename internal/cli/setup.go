package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"harness.engineering/internal/local"
	"harness.engineering/internal/menubar"
	"harness.engineering/internal/model"
)

func setup(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) error {
	flags := newFlags("setup", diagnostics, " [options]")
	var address, database string
	resolveAddress := serverFlags(flags, &address, &database)
	start := flags.Bool("start", false, "install and start the macOS background server")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if *start {
		if runtime.GOOS != "darwin" {
			return errors.New("launchd is available on macOS. Run backlog serve to start the server here")
		}
		return setupLaunchAgent(ctx, address, database, out)
	}
	if input == nil {
		return errors.New("setup requires answers on standard input")
	}

	answers := bufio.NewScanner(input)
	for {
		fmt.Fprint(out, "Set up Backlog as a client or server? [client/server] ")
		role, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return err
		}
		switch strings.ToLower(role) {
		case "c", "client":
			return setupClient(ctx, answers, address, out)
		case "s", "server":
			return reviewServerSetup(ctx, address, out, diagnostics)
		}
		fmt.Fprintln(out, "Enter client or server.")
	}
}

func setupClient(ctx context.Context, answers *bufio.Scanner, address string, out io.Writer) error {
	path, err := dashboardURLPath()
	if err != nil {
		return err
	}
	settings, err := readClientSettings()
	if err != nil {
		return err
	}
	localAddress, err := loopbackBindAddress(address)
	if err != nil {
		localAddress = local.DefaultAddress
	}
	current, err := dashboardURL("http://" + localAddress + "/")
	if err != nil {
		return err
	}
	if _, err := parseDashboardURL(current); err != nil {
		current = "http://" + localAddress + "/"
	}
	for {
		fmt.Fprintf(out, "BACKLOG_URL [%s] ", current)
		value, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return err
		}
		if value == "" {
			value = current
		}
		if _, err := parseDashboardURL(value); err != nil {
			fmt.Fprintln(out, err)
			continue
		}
		settings.DashboardURL = value
		break
	}
	for {
		fmt.Fprint(out, "BACKLOG_USERNAME (optional, leave blank to skip) ")
		value, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return err
		}
		if !model.ValidMonitorName(value) {
			fmt.Fprintln(out, "Enter a username of up to 48 printable characters.")
			continue
		}
		settings.Username = value
		break
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := saveClientSettings(path, settings); err != nil {
		return fmt.Errorf("save client settings (%w)", err)
	}
	fmt.Fprintf(out, "Saved BACKLOG_URL=%s\n", settings.DashboardURL)
	if settings.Username != "" {
		fmt.Fprintf(out, "Saved BACKLOG_USERNAME=%s\n", settings.Username)
	}
	if override := strings.TrimSpace(os.Getenv("BACKLOG_URL")); override != "" && override != settings.DashboardURL {
		fmt.Fprintln(out, "The current BACKLOG_URL environment variable takes precedence.")
	}
	if backlogUsername(settings.Username) != settings.Username {
		fmt.Fprintln(out, "The current BACKLOG_USERNAME environment variable takes precedence.")
	}
	if menubar.Available {
		fmt.Fprintln(out, "Run backlog menu to monitor projects from the macOS menu bar.")
	}
	return nil
}

func reviewServerSetup(ctx context.Context, address string, out, diagnostics io.Writer) error {
	probe, err := loopbackBindAddress(address)
	if err != nil {
		return err
	}
	report := inspectStatus(ctx, probe, "http://"+probe, diagnostics)
	if err := ctx.Err(); err != nil {
		return err
	}
	report.Advice = serverSetupAdvice(report)
	return writeStatus(out, probe, report)
}

func serverSetupAdvice(report statusReport) string {
	switch {
	case report.ServerVersion != "" && strings.HasPrefix(report.Background, "registered"):
		return "Backlog is running and a launchd job is registered. Run backlog restart for the restart command."
	case report.ServerVersion != "":
		return "Backlog is ready. To switch to launchd on macOS, stop this instance and run backlog setup -start."
	case report.Port == "listening":
		return "Another listener occupies this port. Choose another port with -p or stop the listener before starting Backlog."
	case report.Port == "check unavailable":
		return "Check the local address and port, then run backlog setup again."
	case runtime.GOOS != "darwin":
		return "Run backlog serve to start the server."
	case report.Background == "no Backlog launch agent detected":
		return "Run backlog setup -start to install and start the macOS background server."
	default:
		return "Run backlog restart for the launchd start or restart command."
	}
}

func readSetupAnswer(ctx context.Context, answers *bufio.Scanner) (string, error) {
	type response struct {
		text string
		err  error
	}
	read := make(chan response, 1)
	// Let Ctrl-C cancel the prompt while the read is blocked on stdin.
	go func() {
		if answers.Scan() {
			read <- response{text: strings.TrimSpace(answers.Text())}
		} else if err := answers.Err(); err != nil {
			read <- response{err: err}
		} else {
			read <- response{err: errors.New("setup ended before all questions were answered")}
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case answer := <-read:
		return answer.text, answer.err
	}
}
