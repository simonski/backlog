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

To allow archiving and project color changes through an existing Tailscale Serve HTTPS route, run Backlog on loopback and name the exact route host and the permitted Tailscale login. Include the port in the host when the HTTPS URL uses a nonstandard port.

```sh
backlog serve -addr 127.0.0.1:15000 \
  -tailscale-archive-host mini.example.ts.net:15001 \
  -tailscale-archive-user owner@example.com
```

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

## Homepage activity

The activity grid defaults to four weeks, including today. The 1w, 2w, and 4w buttons are always available. Longer ranges become available as history accumulates. Short ranges show days and the three-month, six-month, and one-year ranges show weeks. Each pixel represents one ticket using its latest recorded status in that period, and occupied cells use the project's pastel tint. Hover over a cell for dates, project, and status counts. The grid uses the server's local time zone.

Projects with Doing tickets appear first. Within the active and inactive groups, the latest ticket status change, report, archive, or document upload determines the order. A document upload temporarily raises the project to the top for six seconds. Cards slide into place when motion is enabled.

Right-click a project card to choose a pastel color for its marker and heatmap. When a preset is already assigned, the previous project receives a free preset or its automatic color.

## Project board

A ticket in **Doing** shows a live `hh:mm:ss` timer. Leaving **Doing** pauses the timer, returning resumes it, and **Done** shows the total time spent in **Doing**. Tickets already in **Doing** start tracking at upgrade. A Done ticket with no timing history shows **Total time not recorded**.

A red badge beside the project name shows the number of blocked tickets. With archive access, right-click a project card or ticket and select **Archive**. Right-click a **Blocked**, **To do**, **Done**, or **Rejected** column and choose **Archive all**. Use **Shift+F10** on a focused card, ticket, or column to open its menu by keyboard. Archive actions ask for confirmation. Archived items stay out of the dashboard and activity grid, and the CLI still lists their tickets.

The homepage's **Done** count is the lifetime number of distinct completed tickets, including archived completions.

## Project documents

Publish Markdown from the project directory. Each document appears as a link above the ticket board. The homepage card also links directly to a README or specification when one exists. The first upload creates the project if needed. Upload under the same name to replace the document.

```sh
backlog doc -file README.md
backlog doc -file SPECIFICATION.md
backlog doc -name "Project overview" -file docs/overview.md
backlog doc -name "Release notes" -file - < RELEASE.md
```

The file name is the default document name. Give documents their original file names if they link to each other in Markdown. Use `-name` when reading Markdown from stdin. The upload request can be up to 1 MiB. The command supports `-json`, `-p`, and `-addr`.

Run `backlog doc -clear` from a project's root to remove all its uploaded documents. It preserves the source files, project, and tickets.

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
