package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func installSkill(args []string, contents []byte, out, diagnostics io.Writer) error {
	flags := newFlags("install-skill", diagnostics, " [-dir SKILLS_DIRECTORY]")
	directory := flags.String("dir", "", "skills directory (defaults to ~/.agents/skills)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if len(contents) == 0 {
		return errors.New("embedded reporting skill is unavailable")
	}
	if *directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*directory = filepath.Join(home, ".agents", "skills")
	}
	skillDir, err := filepath.Abs(filepath.Join(*directory, "report-harness-progress"))
	if err != nil {
		return err
	}
	path := filepath.Join(skillDir, "SKILL.md")
	installed, err := writeSkill(path, contents)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Path      string `json:"path"`
		Installed bool   `json:"installed"`
	}{path, installed})
}

func writeSkill(path string, contents []byte) (bool, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, err
	}
	staged, err := os.CreateTemp(directory, ".skill-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(staged.Name())
	_, writeErr := staged.Write(contents)
	if err := errors.Join(writeErr, staged.Close()); err != nil {
		return false, err
	}

	// Publish the complete file atomically while preserving any existing copy.
	err = os.Link(staged.Name(), path)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode().IsRegular() && info.Size() == int64(len(contents)) {
		existing, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if bytes.Equal(existing, contents) {
			return false, nil
		}
	}
	return false, fmt.Errorf("a different skill already exists at %s", path)
}
