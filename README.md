# Backlog

Backlog is a live dashboard for tracking the work an agentic harness plans, starts, blocks on, and completes.

## Install and run

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install simonski/backlog/backlog
backlog setup
```

On macOS, the wizard asks whether to install and start a background service and whether to install the reporting skill. The service runs as the current user, starts at login, and restarts if it exits. If a foreground `backlog serve` is already running, stop it with Ctrl-C before starting the service. Run setup as the same user to reuse the default SQLite database. Pass `-p 15001` for a different port or `-db PATH` to reuse a custom database.

Open [127.0.0.1:15000](http://127.0.0.1:15000). From another device on your tailnet, open `http://<server-tailscale-ip>:15000` for a read-only view.

Project summaries and task boards show statuses in this order **Blocked, To do, Doing, Done, No longer needed**.

```sh
launchctl print "gui/$(id -u)/local.backlog"    # status
tail -f "$HOME/Library/Logs/backlog.log"       # logs
launchctl bootout "gui/$(id -u)/local.backlog" # stop until next login
```

To remove the service from future logins, unload it and remove `~/Library/LaunchAgents/local.backlog.plist`. Run `backlog serve` for foreground operation. On other platforms, the wizard offers skill installation and the server can be started with `backlog serve`.

## Reporting skill

The wizard installs `report-harness-progress` under `~/.agents/skills`. Ask your harness to use that skill from the project directory. For a project-only install, run `backlog setup -dir .agents/skills` from that project. For port 15001, set `HARNESS_ADDR=127.0.0.1:15001` in the harness environment.

To install just the skill without prompts, run:

```sh
backlog install-skill
```

`backlog install-skill` also accepts `-dir`.

## Upgrade

Run `brew upgrade simonski/backlog/backlog`. A server started through the Homebrew executable link restarts automatically, and open dashboards reload.
