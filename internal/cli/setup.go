package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
)

func setup(ctx context.Context, args []string, skill []byte, input io.Reader, out, diagnostics io.Writer) error {
	flags := newFlags("setup", diagnostics, " [options]")
	var address, database string
	resolveAddress := serverFlags(flags, &address, &database)
	directory := flags.String("dir", "", "skills directory (defaults to ~/.agents/skills)")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if input == nil {
		return errors.New("setup requires answers on standard input")
	}

	answers := bufio.NewScanner(input)
	var service bool
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(out, "The background service runs as the current user, starts at login, and restarts if it exits.")
		fmt.Fprintln(out, "If backlog serve is running in another terminal, stop it before starting the service.")
		var err error
		service, err = confirmSetup(ctx, answers, out, "Install and start the macOS background service?")
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "Background service setup is available on macOS. Use backlog serve to start the server here.")
	}
	skillDirectory := *directory
	if skillDirectory == "" {
		skillDirectory = "~/.agents/skills"
	}
	reporting, err := confirmSetup(ctx, answers, out, fmt.Sprintf("Install the Backlog skill in %s?", skillDirectory))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var serviceErr, skillErr error
	if service {
		serviceErr = resolveAddress()
		if serviceErr == nil {
			serviceErr = setupLaunchAgent(ctx, address, database, out)
		}
		if serviceErr != nil {
			serviceErr = fmt.Errorf("set up the background service: %w", serviceErr)
		}
	}
	if reporting && ctx.Err() == nil {
		path, installed, err := ensureSkill(*directory, skill)
		if err != nil {
			skillErr = fmt.Errorf("set up the reporting skill: %w", err)
		} else {
			skillErr = printSkillInstallation(out, path, installed)
			fmt.Fprintln(out, "Configure the harness to use the backlog skill from the project directory.")
		}
	}
	return errors.Join(serviceErr, skillErr, ctx.Err())
}

func confirmSetup(ctx context.Context, answers *bufio.Scanner, out io.Writer, question string) (bool, error) {
	type response struct {
		text string
		err  error
	}
	for {
		fmt.Fprintf(out, "%s [Y/n] ", question)
		// Let Ctrl-C cancel the prompt while the read is blocked on stdin.
		read := make(chan response, 1)
		go func() {
			if answers.Scan() {
				read <- response{text: strings.ToLower(strings.TrimSpace(answers.Text()))}
			} else if err := answers.Err(); err != nil {
				read <- response{err: err}
			} else {
				read <- response{err: errors.New("setup ended before all questions were answered")}
			}
		}()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case answer := <-read:
			if answer.err != nil {
				return false, answer.err
			}
			switch answer.text {
			case "", "y", "yes":
				return true, nil
			case "n", "no":
				return false, nil
			}
		}
		fmt.Fprintln(out, "Enter yes or no.")
	}
}
