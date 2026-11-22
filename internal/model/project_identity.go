package model

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type ProjectIdentity struct {
	Repository string `json:"repository,omitempty"`
	Path       string `json:"path"`
}

type ProjectInitResult struct {
	ProjectID  int64  `json:"project_id"`
	Identifier string `json:"identifier"`
}

var githubPath = regexp.MustCompile(`^/[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$`)

func GitHubRepository(remote string) string {
	scpHost, scpPath, scp := strings.Cut(remote, ":")
	switch {
	case scp && strings.EqualFold(scpHost, "git@github.com"):
		remote = "ssh://git@github.com/" + strings.TrimLeft(scpPath, "/")
	}
	parsed, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") ||
		(parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh" && parsed.Scheme != "git") {
		return ""
	}
	repository := strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), ".git")
	if !githubPath.MatchString(repository) {
		return ""
	}
	return "https://github.com" + repository
}

func PortablePath(root, home string) string {
	if home != "" {
		if relative, err := filepath.Rel(home, root); err == nil && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if relative == "." {
				return "~"
			}
			return "~/" + filepath.ToSlash(relative)
		}
	}
	return filepath.ToSlash(root)
}

func ResolveProjectPath(path, home string) (string, error) {
	switch {
	case path == "~" || strings.HasPrefix(path, "~/"):
		if home == "" {
			return "", Invalid("the server home directory is unavailable")
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/")
		root := filepath.Join(home, filepath.FromSlash(relative))
		if PortablePath(root, home) != path {
			return "", Invalid("project path must stay inside the home directory")
		}
		return root, nil
	default:
		root := filepath.FromSlash(path)
		return root, ValidateRoot(root)
	}
}
