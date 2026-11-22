package cli

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"harness.engineering/internal/menubar"
)

func dashboardURLPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "harness-engineering", "backlog-client.json"), nil
}

func readClientSettings() (menubar.Settings, error) {
	var settings menubar.Settings
	path, err := dashboardURLPath()
	if err != nil {
		return settings, err
	}
	stored, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		legacy, err := os.ReadFile(filepath.Join(filepath.Dir(path), "backlog-url"))
		if errors.Is(err, os.ErrNotExist) {
			return settings, nil
		}
		settings.DashboardURL = strings.TrimSpace(string(legacy))
		return settings, err
	}
	if err != nil {
		return settings, err
	}
	err = json.Unmarshal(stored, &settings)
	return settings, err
}

func dashboardURL(fallback string) (string, error) {
	if value := strings.TrimSpace(os.Getenv("BACKLOG_URL")); value != "" {
		return value, nil
	}
	settings, err := readClientSettings()
	if settings.DashboardURL == "" {
		return fallback, err
	}
	return settings.DashboardURL, err
}

func backlogUsername(saved string) string {
	if value, present := os.LookupEnv("BACKLOG_USERNAME"); present {
		return strings.TrimSpace(value)
	}
	return saved
}

func parseDashboardURL(value string) (*url.URL, error) {
	address, err := url.Parse(value)
	if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Hostname() == "" ||
		address.User != nil || address.RawQuery != "" || address.ForceQuery || address.Fragment != "" {
		return nil, errors.New("enter an HTTP or HTTPS dashboard URL without credentials, query parameters, or a fragment")
	}
	return address, nil
}

func saveClientSettings(path string, settings menubar.Settings) error {
	content, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	staged, err := stageFile(directory, append(content, '\n'))
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return os.Rename(staged, path)
}
