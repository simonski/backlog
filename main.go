package main

import (
	"context"
	_ "embed"
	"os"
	"os/signal"
	"syscall"

	"harness.engineering/internal/cli"
)

//go:embed .agents/skills/backlog/SKILL.md
var reportingSkill []byte

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := cli.App{ReportingSkill: reportingSkill}
	os.Exit(app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
