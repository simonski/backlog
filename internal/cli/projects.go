package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
	"harness.engineering/internal/wire"
)

const projectUsage = `Usage: backlog project <command> [options]

Commands:
  ls    List tracked projects and their IDs
  init  Register the current project by GitHub repository or path

Use backlog ls -project N to list tickets for a project.
`

func projectCommand(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	if len(args) == 0 || args[0] == "-help" || args[0] == "-h" || args[0] == "help" {
		_, err := fmt.Fprint(out, projectUsage)
		return err
	}
	switch args[0] {
	case "init":
		return initProject(ctx, args[1:], out, diagnostics)
	case "ls":
		return listProjects(ctx, args[1:], out, diagnostics)
	default:
		return fmt.Errorf("unknown project command %q (run backlog project -help)", args[0])
	}
}

func listProjects(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("project ls", diagnostics, " [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "override the dashboard with a local host:port")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := resolveAddress(); err != nil {
		return err
	}
	projects, err := fetchProjects(ctx, flags, address, nil, diagnostics)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, projects)
	}
	if len(projects) == 0 {
		_, err := fmt.Fprintln(out, "No projects tracked.")
		return err
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tPROJECT\tDOING\tTODO\tBLOCKED\tDONE\tREJECTED\tIDENTIFIER")
	for _, project := range projects {
		identifier := project.Repository
		if identifier == "" {
			identifier = project.Root
		}
		fmt.Fprintf(table, "%d\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n",
			project.ID, ticketCell(projectLabel(project)), project.Doing, project.Todo, project.Blocked,
			project.Done, project.Rejected, ticketCell(identifier))
	}
	return table.Flush()
}

func initProject(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("project init", diagnostics, " [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	var address string
	resolveAddress := addressFlags(flags, &address, local.DefaultAddress, "local server host:port")
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
	identity := model.ProjectIdentity{Repository: githubRemote(ctx, root), Path: model.PortablePath(root, canonicalHome())}
	var project model.ProjectInitResult
	if err := requestServer(ctx, address, http.MethodPost, "/api/projects", identity, &project, diagnostics); err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, project)
	}
	_, err = fmt.Fprintf(out, "Project %d  %s\n", project.ProjectID, ticketCell(project.Identifier))
	return err
}

func projectLabel(project model.ProjectListing) string {
	if project.Archived {
		return project.Name + " (archived)"
	}
	return project.Name
}

func fetchProjects(ctx context.Context, flags *flag.FlagSet, address string, query url.Values, diagnostics io.Writer) ([]model.ProjectListing, error) {
	settings, err := readClientSettings()
	if err != nil {
		return nil, fmt.Errorf("read client settings (%w)", err)
	}
	base := "http://" + address
	if hasAddressOverride(flags) {
		if err := local.ValidateAddress(address); err != nil {
			return nil, err
		}
	} else {
		base, err = dashboardURL(base)
		if err != nil {
			return nil, fmt.Errorf("read BACKLOG_URL (%w)", err)
		}
	}
	if _, err := parseDashboardURL(base); err != nil {
		return nil, err
	}
	base = strings.TrimRight(base, "/")
	client := serverHTTPClient()
	defer client.CloseIdleConnections()
	versionAvailable, err := checkServerVersion(ctx, client, base, diagnostics)
	if err != nil {
		return nil, err
	}
	var projects []model.ProjectListing
	endpoint := base + "/api/projects"
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	headers := make(http.Header)
	if name := backlogUsername(settings.Username); name != "" && model.ValidMonitorName(name) {
		headers.Set(wire.MonitorUsernameHeader, name)
	}
	err = requestWithHeaders(ctx, client, http.MethodGet, endpoint, nil, &projects, 0, headers)
	if err != nil && !versionAvailable {
		err = fmt.Errorf("%w (server version unavailable)", err)
	}
	return projects, err
}

func hasAddressOverride(flags *flag.FlagSet) bool {
	var explicit bool
	flags.Visit(func(flag *flag.Flag) {
		explicit = explicit || flag.Name == "addr" || flag.Name == "p"
	})
	return explicit
}

func projectHints(ctx context.Context, query url.Values) {
	directory, err := os.Getwd()
	if err != nil {
		return
	}
	if canonical, err := filepath.EvalSymlinks(directory); err == nil {
		directory = canonical
	}
	home := canonicalHome()
	query.Set("directory", model.PortablePath(directory, home))
	root := gitValue(ctx, directory, "rev-parse", "--show-toplevel")
	if root == "" {
		return
	}
	query.Set("git_root", model.PortablePath(root, home))
	if common := gitValue(ctx, directory, "rev-parse", "--path-format=absolute", "--git-common-dir"); filepath.Base(common) == ".git" {
		query.Set("common_root", model.PortablePath(filepath.Dir(common), home))
	}
	if repository := githubRemote(ctx, root); repository != "" {
		query.Set("repository", repository)
	} else if remote := gitValue(ctx, root, "remote", "get-url", "origin"); remote != "" {
		// Send only the repository name. Remote URLs can contain credentials.
		if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
			remote = parsed.Path
		} else if _, after, ok := strings.Cut(remote, ":"); ok {
			remote = after
		}
		query.Set("repository", strings.TrimSuffix(path.Base(strings.TrimRight(remote, "/")), ".git"))
	}
}

func gitValue(ctx context.Context, directory string, args ...string) string {
	result, err := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(result), "\n")
}

func githubRemote(ctx context.Context, directory string) string {
	remotes := append([]string{"origin"}, strings.Split(gitValue(ctx, directory, "remote"), "\n")...)
	for index, name := range remotes {
		if name == "" || (index > 0 && name == "origin") {
			continue
		}
		if repository := model.GitHubRepository(gitValue(ctx, directory, "remote", "get-url", "--", name)); repository != "" {
			return repository
		}
	}
	return ""
}

func canonicalHome() string {
	home, _ := os.UserHomeDir()
	if canonical, err := filepath.EvalSymlinks(home); home != "" && err == nil {
		return canonical
	}
	return home
}
