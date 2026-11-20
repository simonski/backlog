# Backlog

Backlog is a live dashboard for tracking the work an agentic harness plans, starts, blocks on, and completes.

## Install and run

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install simonski/backlog/backlog
backlog setup
```

On macOS, the wizard asks whether to install and start a background service and whether to install the reporting skill. The service runs as the current user, starts at login, and restarts if it exits. If a foreground `backlog serve` is already running, stop it with Ctrl-C before starting the service. Run setup as the same user to reuse the default SQLite database. Pass `-p 15001` for a different port or `-db PATH` to reuse a custom database.

Open [127.0.0.1:15000](http://127.0.0.1:15000). From another device on the same tailnet, open `http://<server-tailscale-ip>:15000` for a read-only view.

Project summaries and task boards show **Blocked**, **To do**, **Doing**, **Done**, and **Rejected**, in that order. A project-wide snapshot marks unfinished tasks it omits as **Rejected**. Reporting a rejected task again returns it to the active task board.

```sh
launchctl print "gui/$(id -u)/local.backlog"    # status
tail -f "$HOME/Library/Logs/backlog.log"       # logs
launchctl bootout "gui/$(id -u)/local.backlog" # stop until next login
```

To remove the service from future logins, unload it and remove `~/Library/LaunchAgents/local.backlog.plist`. Run `backlog serve` for foreground operation. On other platforms, the wizard offers skill installation and the server can be started with `backlog serve`.

From a source checkout, run `make` to list targets. Use `make clean build install` to rebuild and install, or `make run` to build and start the server. Select a different port with `make run -- -p 15001` or `backlog serve -p 15001`.

## macOS menu bar

Run `backlog menu` from a macOS desktop session to show a ticket icon in the menu bar. **Open Backlog** opens the dashboard in the default browser. **Quick Backlog** opens the live dashboard in a compact window.

The menu uses the running local server. For a custom port, use `backlog menu -p 15001`. Press Ctrl-C in the terminal running the command to remove the icon.

## CLI output

`task`, `snapshot`, `install-skill`, and `version` print readable summaries by default. Pass `-json` to receive indented JSON on stdout. Diagnostics, including version warnings, go to stderr.

```sh
backlog version
backlog version -json
backlog task -id sign-in -title "Add sign-in" -status doing
backlog snapshot -file tasks.json -json
```

## List and update tickets

Run these commands from the project directory or any subdirectory of its Git working tree. Use the ticket IDs shown by `ls`. Numeric IDs and names such as `sign-in` both work.

```sh
backlog ls
backlog ls -id 12,15 -json
backlog update -id 12,15 -status doing -progress 25
backlog block -id 12 -blocker "Waiting for review"
backlog todo -id 12,15
backlog doing -id 12,15
backlog done -id 12,15
```

`ls` shows every ticket by default, including completed and rejected tickets. `update` applies any supplied `-title`, `-description`, `-status`, `-progress`, or `-blocker` fields to each selected ticket. Use `-blocker ''` to clear a blocker note. Each change applies to the entire list in one transaction. An unknown ID causes the command to fail before any changes are committed. Use `task -id ID -title TITLE` to create a ticket.

All six commands support `-json` and the server connection options `-p` and `-addr`.

Snapshot files and snapshot input on stdin use JSON in either output mode.

## Reporting skill

The wizard installs the `backlog` skill at `~/.agents/skills/backlog/SKILL.md`. Configure the harness to use that skill from the project directory. For a project-only install, run `backlog setup -dir .agents/skills` from that project. For port 15001, set `HARNESS_ADDR=127.0.0.1:15001` in the harness environment.

To install just the skill without prompts, run:

```sh
backlog install-skill
```

`backlog install-skill` also accepts `-dir`.

For upgrades from `report-harness-progress`, install `backlog`, carry over any customizations, and retire the old skill directory to avoid loading both skills.

## Upgrade

Run `brew upgrade simonski/backlog/backlog`. A server started through the Homebrew executable link restarts automatically, and open dashboards reload.
