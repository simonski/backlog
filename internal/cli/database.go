package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"harness.engineering/internal/storage"
)

func databasePath(path string) (string, error) {
	if path == "" {
		settings, err := readServerSettings()
		if err != nil {
			return "", err
		}
		path = settings.Database
	}
	if path == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate user configuration directory: %w", err)
		}
		path = filepath.Join(config, "harness-engineering", "telemetry.db")
	}
	return filepath.Abs(path)
}

func initDB(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("initdb", diagnostics, " [-db PATH]")
	database := flags.String("db", "", "SQLite database path")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	path, err := databasePath(*database)
	if err != nil {
		return err
	}
	return initializeDatabase(ctx, path, out)
}

func initializeDatabase(ctx context.Context, path string, out io.Writer) error {
	store, err := storage.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("initialize SQLite at %s: %w", path, err)
	}
	if err := store.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Database ready at %s\n", path)
	return err
}
