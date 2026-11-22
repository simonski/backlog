package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func servePIDs(ctx context.Context) ([]int, error) {
	processes, err := exec.CommandContext(ctx, "ps", "-ww", "-axo", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(string(processes), "\n") {
		pidText, executable, found := strings.Cut(strings.TrimSpace(line), " ")
		executable = strings.TrimSpace(executable)
		pid, err := strconv.Atoi(pidText)
		if !found || err != nil || pid == os.Getpid() || filepath.Base(executable) != "backlog" {
			continue
		}
		args, err := exec.CommandContext(ctx, "ps", "-ww", "-p", pidText, "-o", "args=").Output()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil && isServeProcess(executable, string(args)) {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func isServeProcess(executable, commandLine string) bool {
	commandLine = strings.TrimSpace(commandLine)
	if args, ok := strings.CutPrefix(commandLine, executable+" "); ok {
		fields := strings.Fields(args)
		return len(fields) > 0 && fields[0] == "serve"
	}
	fields := strings.Fields(commandLine)
	return len(fields) > 1 && filepath.Base(fields[0]) == "backlog" && fields[1] == "serve"
}

func backgroundServiceStatus(ctx context.Context) string {
	if runtime.GOOS != "darwin" {
		return "launchd not applicable"
	}
	for _, target := range launchAgentTargets() {
		if err := exec.CommandContext(ctx, "/bin/launchctl", "print", target).Run(); err == nil {
			return "registered (" + filepath.Base(target) + ")"
		}
	}
	if ctx.Err() != nil {
		return "launchd check unavailable"
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")); err == nil {
			return "launch agent installed, not registered"
		}
	}
	return "no Backlog launch agent detected"
}

func launchAgentTargets() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	domain := "gui/" + strconv.Itoa(os.Getuid()) + "/"
	return []string{domain + launchAgentLabel, domain + launchAgentLabel + ".session"}
}
