# Backlog

Backlog is a live dashboard for tracking the work an agentic harness plans, starts, blocks on, and completes.

## Install and run

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install simonski/backlog/backlog
backlog serve
```

Open [127.0.0.1:15000](http://127.0.0.1:15000). From another device on your tailnet, open `http://<server-tailscale-ip>:15000` for a read-only view. To use a different port, run `backlog serve -p 15001`.

## Install the reporting skill

In another terminal on the server, run:

```sh
backlog install-skill
```

This installs `report-harness-progress` under `~/.agents/skills`. Ask your harness to use that skill from the project directory. For a project-only install, run `backlog install-skill -dir .agents/skills` from that project. For port 15001, set `HARNESS_ADDR=127.0.0.1:15001` in the harness environment.

## Upgrade

Run `brew upgrade simonski/backlog/backlog`. A server started through the Homebrew executable link restarts automatically, and open dashboards reload.
