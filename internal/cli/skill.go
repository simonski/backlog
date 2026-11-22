package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func installSkill(args []string, contents []byte, out, diagnostics io.Writer) error {
	flags := newFlags("install-skill", diagnostics, " [options]")
	asJSON := flags.Bool("json", false, "print indented JSON")
	directory := flags.String("dir", "", "skills directory (defaults to ~/.agents/skills)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	path, installed, err := ensureSkill(*directory, contents)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(out, struct {
			Path      string `json:"path"`
			Installed bool   `json:"installed"`
		}{path, installed})
	}
	return printSkillInstallation(out, path, installed)
}

func printSkillInstallation(out io.Writer, path string, installed bool) error {
	if installed {
		_, err := fmt.Fprintf(out, "Installed reporting skill at %s\n", path)
		return err
	}
	_, err := fmt.Fprintf(out, "Reporting skill already installed at %s\n", path)
	return err
}

func ensureSkill(directory string, contents []byte) (string, bool, error) {
	if len(contents) == 0 {
		return "", false, errors.New("embedded reporting skill is unavailable")
	}
	path, err := skillPath(directory)
	if err != nil {
		return "", false, err
	}
	installed, err := installFile(path, contents)
	if errors.Is(err, os.ErrExist) {
		err = fmt.Errorf("a different skill already exists at %s: %w", path, err)
	}
	return path, installed, err
}

func skillPath(directory string) (string, error) {
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".agents", "skills")
	}
	return filepath.Abs(filepath.Join(directory, "backlog", "SKILL.md"))
}
