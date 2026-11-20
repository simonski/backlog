package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func installSkill(args []string, contents []byte, out, diagnostics io.Writer) error {
	flags := newFlags("install-skill", diagnostics, " [-dir SKILLS_DIRECTORY]")
	directory := flags.String("dir", "", "skills directory (defaults to ~/.agents/skills)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	path, installed, err := ensureSkill(*directory, contents)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Path      string `json:"path"`
		Installed bool   `json:"installed"`
	}{path, installed})
}

func ensureSkill(directory string, contents []byte) (string, bool, error) {
	if len(contents) == 0 {
		return "", false, errors.New("embedded reporting skill is unavailable")
	}
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, err
		}
		directory = filepath.Join(home, ".agents", "skills")
	}
	path, err := filepath.Abs(filepath.Join(directory, "report-harness-progress", "SKILL.md"))
	if err != nil {
		return "", false, err
	}
	installed, err := installFile(path, contents)
	if errors.Is(err, os.ErrExist) {
		err = fmt.Errorf("a different skill already exists at %s", path)
	}
	return path, installed, err
}
