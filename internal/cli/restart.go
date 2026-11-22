package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func showRestart(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	flags := newFlags("restart", diagnostics, "")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	targets := launchAgentTargets()
	if len(targets) == 0 {
		return errors.New("launchd restart advice is available on macOS. Restart Backlog through its supervisor or use backlog serve -stop followed by backlog serve")
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var registered []string
	for _, target := range targets {
		if exec.CommandContext(lookup, "/bin/launchctl", "print", target).Run() == nil {
			registered = append(registered, target)
		}
	}
	if err := lookup.Err(); err != nil {
		return fmt.Errorf("check Backlog launchd registration (%w)", err)
	}
	command, err := restartCommand(registered, filepath.Dir(targets[0]))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, command)
	return err
}

func restartCommand(registered []string, domain string) (string, error) {
	switch len(registered) {
	case 1:
		return "launchctl kickstart -k " + registered[0], nil
	case 0:
	default:
		return "", fmt.Errorf("multiple Backlog launchd jobs are registered (%s). Use launchctl kickstart -k with the intended job", strings.Join(registered, ", "))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	if _, err := os.Stat(plist); err == nil {
		return "launchctl bootstrap " + domain + " " + shellArgument(plist), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	executable, err := executablePath()
	if err != nil {
		return "", err
	}
	return shellArgument(executable) + " setup", nil
}

func shellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
