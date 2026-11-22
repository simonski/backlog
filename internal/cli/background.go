package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

const serverReadyFD = "BACKLOG_SERVER_READY_FD"

type startupResult struct {
	Address string
	Error   string
}

func startBackground(ctx context.Context, args []string, out io.Writer) error {
	executable, err := executablePath()
	if err != nil {
		return err
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	directory = filepath.Join(directory, "harness-engineering")
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		directory = filepath.Join(home, "Library", "Logs")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(directory, "backlog.log")
	log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	access, err := os.OpenFile(accessLogPath(logPath), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer access.Close()
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	command := exec.Command(executable, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Stdout, command.Stderr = access, log
	command.ExtraFiles = []*os.File{writer}
	command.Env = append(os.Environ(), serverReadyFD+"=3", serverLockFD+"=")
	if err := command.Start(); err != nil {
		return err
	}
	writer.Close()
	ready := make(chan startupResult, 1)
	go func() {
		var result startupResult
		if err := json.NewDecoder(io.LimitReader(reader, 8192)).Decode(&result); err != nil {
			result.Error = fmt.Sprintf("background server exited before reporting readiness: %v (see %s)", err, logPath)
		}
		ready <- result
	}()
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	select {
	case result := <-ready:
		if result.Error != "" {
			_ = command.Wait()
			return errors.New(result.Error)
		}
		pid := command.Process.Pid
		_ = command.Process.Release()
		_, err := fmt.Fprintf(out, "Backlog is running in the background (PID %d).\nDashboard: http://%s\nError log: %s\nAccess log: %s\n", pid, result.Address, logPath, accessLogPath(logPath))
		return err
	case <-startup.Done():
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("waiting for the background server (see %s): %w", logPath, startup.Err())
	}
}
