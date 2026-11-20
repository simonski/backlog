package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

// installFile publishes a complete file atomically and preserves existing files.
func installFile(path string, contents []byte) (bool, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, err
	}
	staged, err := os.CreateTemp(directory, ".backlog-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(staged.Name())
	_, writeErr := staged.Write(contents)
	if err := errors.Join(writeErr, staged.Close()); err != nil {
		return false, err
	}

	err = os.Link(staged.Name(), path)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, os.ErrExist) {
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
	return false, os.ErrExist
}
