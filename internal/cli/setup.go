package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"harness.engineering/internal/menubar"
	"harness.engineering/internal/model"
	"harness.engineering/internal/storage"
)

func (app App) setup(ctx context.Context, args []string, input io.Reader, out, diagnostics io.Writer) error {
	flags := newFlags("setup", diagnostics, " [options]")
	var address, database string
	resolveAddress := serverFlags(flags, &address, &database)
	start := flags.Bool("start", false, "install and start the macOS background server")
	skillDirectory := flags.String("dir", "", "skills directory (defaults to ~/.agents/skills)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	if *start {
		if runtime.GOOS != "darwin" {
			return errors.New("launchd is available on macOS. Run backlog serve -d to start a background server here")
		}
		settings, err := readServerSettings()
		if err != nil {
			return err
		}
		if database == "" {
			database = settings.Database
		}
		return app.runtime.setupLaunchAgent(ctx, address, database, out)
	}
	if input == nil {
		return errors.New("setup requires answers on standard input")
	}

	client, err := readClientSettings()
	if err != nil {
		return err
	}
	server, err := readServerSettings()
	if err != nil {
		return err
	}
	path, err := databasePath(database)
	if err != nil {
		return err
	}
	checkDB, cancel := context.WithTimeout(ctx, 2*time.Second)
	initialized, dbErr := storage.Initialized(checkDB, path)
	cancel()
	skill, err := skillPath(*skillDirectory)
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	defaultURL := "http://" + net.JoinHostPort("localhost", port) + "/"
	clientURL, err := dashboardURL(defaultURL)
	if err != nil {
		return err
	}
	if _, err := parseDashboardURL(clientURL); err != nil {
		clientURL = defaultURL
	}
	serverURL := server.DashboardURL
	if serverURL == "" {
		serverURL = defaultURL
	}
	fmt.Fprintln(out, "Current setup")
	fmt.Fprintf(out, "Client URL: %s\nServer URL: %s\n", clientURL, serverURL)
	fmt.Fprintf(out, "Saved client configuration: %t\nSaved server configuration: %t\n", client.DashboardURL != "", server.DashboardURL != "")
	switch _, err := os.Stat(skill); {
	case err == nil:
		fmt.Fprintf(out, "Reporting skill: installed at %s\n", skill)
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(out, "Reporting skill: not installed (%s)\n", skill)
	default:
		fmt.Fprintf(out, "Reporting skill status unavailable: %v\n", err)
	}
	switch {
	case dbErr != nil:
		fmt.Fprintf(out, "Database status unavailable at %s: %v\n", path, dbErr)
	case initialized:
		fmt.Fprintf(out, "Database: initialized at %s\n", path)
	default:
		fmt.Fprintf(out, "Database: not initialized (%s)\n", path)
	}
	if instance, err := app.runtime.control(ctx, http.MethodGet, "/status"); err == nil {
		address = instance.Address
	}
	if err := reviewServerSetup(ctx, address, serverURL, out, diagnostics); err != nil {
		return err
	}
	fmt.Fprintln(out)
	answers := bufio.NewScanner(input)
	for {
		fmt.Fprint(out, "Set up Backlog as a client or server? [client/server] ")
		role, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return err
		}
		switch strings.ToLower(role) {
		case "c", "client":
			return app.setupClient(ctx, answers, client, clientURL, *skillDirectory, out)
		case "s", "server":
			return setupServer(ctx, answers, serverURL, path, database != "" || server.Database != "", initialized, dbErr, out)
		}
		fmt.Fprintln(out, "Enter client or server.")
	}
}

func (app App) setupClient(ctx context.Context, answers *bufio.Scanner, settings menubar.Settings, currentURL, skillDirectory string, out io.Writer) error {
	path, err := dashboardURLPath()
	if err != nil {
		return err
	}
	settings.DashboardURL, err = promptSetupURL(ctx, answers, "BACKLOG_URL", currentURL, out)
	if err != nil {
		return err
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
	install, err := setupYesNo(ctx, answers, "Install the bundled reporting skill?", out)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if install {
		installedPath, installed, err := ensureSkill(skillDirectory, app.ReportingSkill)
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintf(out, "Preserved existing reporting skill at %s\n", installedPath)
		} else if err != nil {
			return err
		} else if err := printSkillInstallation(out, installedPath, installed); err != nil {
			return err
		}
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

func setupServer(ctx context.Context, answers *bufio.Scanner, currentURL, database string, saveDatabase, initialized bool, dbErr error, out io.Writer) error {
	dashboard, err := promptSetupURL(ctx, answers, "BACKLOG_URL for clients", currentURL, out)
	if err != nil {
		return err
	}
	if dbErr == nil && !initialized {
		initialize, err := setupYesNo(ctx, answers, "Initialize the database?", out)
		if err != nil {
			return err
		}
		if initialize {
			if err := initializeDatabase(ctx, database, out); err != nil {
				return err
			}
		}
	}
	path, err := serverSettingsPath()
	if err != nil {
		return err
	}
	settings := serverSettings{DashboardURL: dashboard}
	if saveDatabase {
		settings.Database = database
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := saveSettings(path, settings); err != nil {
		return fmt.Errorf("save server settings: %w", err)
	}
	_, err = fmt.Fprintf(out, "Saved server URL for clients: %s\n", dashboard)
	return err
}

func promptSetupURL(ctx context.Context, answers *bufio.Scanner, label, current string, out io.Writer) (string, error) {
	for {
		fmt.Fprintf(out, "%s [%s] ", label, current)
		value, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = current
		}
		if _, err := parseDashboardURL(value); err != nil {
			fmt.Fprintln(out, err)
			continue
		}
		return value, nil
	}
}

func setupYesNo(ctx context.Context, answers *bufio.Scanner, prompt string, out io.Writer) (bool, error) {
	for {
		fmt.Fprintf(out, "%s [y/N] ", prompt)
		answer, err := readSetupAnswer(ctx, answers)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		}
		fmt.Fprintln(out, "Enter yes or no.")
	}
}

func reviewServerSetup(ctx context.Context, address, dashboard string, out, diagnostics io.Writer) error {
	probe, err := loopbackBindAddress(address)
	if err != nil {
		_, port, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			return splitErr
		}
		probe = net.JoinHostPort("127.0.0.1", port)
	}
	report := inspectStatus(ctx, probe, dashboard, diagnostics)
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
