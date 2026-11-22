# Backlog

Backlog is a live dashboard for tracking the work an agentic harness plans, starts, blocks on, and completes.

## Install and run

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install simonski/backlog/backlog
backlog setup
```

On macOS, the wizard asks whether to install and start a background service and whether to install the reporting skill. The service runs as the current user, starts at login, and restarts if it exits. If a foreground `backlog serve` is already running, stop it with Ctrl-C before starting the service. Run setup as the same user to reuse the default SQLite database. The default port is `15000`. Use `-p N` to select a port or `-db PATH` to reuse a custom database.

Open [http://127.0.0.1:15000/](http://127.0.0.1:15000/) on the machine running Backlog.

Project summaries and task boards show **Blocked**, **To do**, **Doing**, **Done**, and **Rejected**, in that order. A project-wide snapshot marks unfinished tasks it omits as **Rejected**. Reporting a rejected task again returns it to the active task board.

```sh
launchctl print "gui/$(id -u)/local.backlog"    # status
tail -F "$HOME/Library/Logs/backlog.log" "$HOME/Library/Logs/backlog-access.log"
launchctl bootout "gui/$(id -u)/local.backlog" # stop until next login
```

The server writes one access record per HTTP request to stdout. It includes the connection peer, method, path, response status, byte count, and duration. Launchd saves stdout to `~/Library/Logs/backlog-access.log` and stderr to `~/Library/Logs/backlog.log`. Run `backlog setup` again to add the stdout file to a standard launchd configuration created by an older binary.

To remove the service from future logins, unload it and remove `~/Library/LaunchAgents/local.backlog.plist`. Run `backlog serve` for foreground operation. On other platforms, the wizard offers skill installation and the server can be started with `backlog serve`.

From a source checkout, run `make` to list targets. Use `make clean build install` to rebuild and install, or `make run` to build and start the server. To set the port explicitly, use `make run -- -p 15000` or `backlog serve -p 15000`.

If the port is already in use, `backlog serve` exits with an error and identifies an existing Backlog instance when it can. Use `-p N` to start on a different port.

Run `backlog serve -stop` to ask a running local Backlog server to shut down gracefully. Pass the same `-p N` or `-addr` if it uses a custom endpoint. On macOS the command unloads the Backlog launchd job when it owns that listener, so the job cannot immediately restart it. Run `backlog setup` to register the standard background service again.

## Restart the macOS background service

Run `backlog setup` and accept the background service option to run Backlog under launchd. After building a new version, install it at the executable path used by the launchd job. For a job registered from `~/.local/bin/backlog`, use `make install` from the source checkout. A running server watches its launch executable and reloads when the installed binary changes.

Run `backlog restart` to print the launchd command for the current setup. Execute the printed command to restart a registered job or load an installed LaunchAgent. If the service needs initial setup, the command prints the setup invocation.

## Serve Backlog over Tailscale

Use a Tailscale Service named `backlog` to open `https://backlog.<tailnet>.ts.net/` from another tailnet device. Tailscale handles HTTPS on port `443` and forwards to the Backlog server on `127.0.0.1:15000`.

1. Start Backlog on the host and confirm `backlog status` reports it running on port `15000`. On macOS, run `backlog setup` and accept the background service option to keep it running under launchd.
2. In the Tailscale admin console, define a tag such as `tag:backlog-host` in `tagOwners` under **Access controls**. In **Machines**, open the Backlog host's menu, choose **Edit tags**, apply the tag, and save. A Tailscale Service host requires a tag-based identity. Check your access rules when tagging a device that was previously signed in as a user. [Tailscale tags](https://tailscale.com/docs/features/tags)
3. In **Services**, create `svc:backlog` with endpoint `tcp:443`. Check that your tailnet policy allows the intended clients to reach the service.
4. Run this on the tagged host. If `tailscale` is unavailable on PATH on macOS, substitute `/Applications/Tailscale.app/Contents/MacOS/Tailscale`.

   ```sh
   tailscale serve --service=svc:backlog --https=443 http://127.0.0.1:15000
   ```

5. In **Services → backlog → Service hosts**, approve the host if it is pending. Open the HTTPS URL printed by Tailscale from a device on your tailnet. [Tailscale Services setup](https://tailscale.com/docs/features/tailscale-services)

For archive and project-color controls through Tailscale, set the exact service hostname and permitted login in the Backlog startup arguments. Replace the example hostname and login with your values. For a background server, use these arguments in the launchd job.

```sh
backlog serve -addr 127.0.0.1:15000 \
  -tailscale-archive-host backlog.example-tailnet.ts.net \
  -tailscale-archive-user owner@example.com
```

## Check server status

Run `backlog status` or its alias `backlog ps` to see `BACKLOG_URL`, detected `backlog serve` processes, launchd registration on macOS, and whether the configured local port responds as Backlog. A listening port and a healthy Backlog server are reported separately. The command shows the CLI, bundled server, and running server versions and prints upgrade advice on stdout. Compatibility warnings remain on stderr.

```sh
backlog status
backlog status -p 15000 -json
BACKLOG_URL=https://backlog.example-tailnet.ts.net/ backlog status
```

`BACKLOG_URL` controls the displayed dashboard URL and defaults to the local endpoint. `HARNESS_ADDR`, `-addr`, and `-p` select the loopback endpoint to probe. `status` exits nonzero when it cannot confirm a running Backlog server, and `-json` still includes the diagnostic results on stdout.

## macOS menu bar

Run `backlog menu` from a macOS desktop session to show a ticket icon in the menu bar. **Open Backlog** opens the dashboard in the default browser. **Quick Backlog** opens the live dashboard in a compact window.

The menu uses the running local server. To set the port explicitly, use `backlog menu -p 15000`. Press Ctrl-C in the terminal running the command to remove the icon.

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

The wizard installs the `backlog` skill at `~/.agents/skills/backlog/SKILL.md`. Configure the harness to use that skill from the project directory. For a project-only install, run `backlog setup -dir .agents/skills` from that project. To set the address explicitly, use `HARNESS_ADDR=127.0.0.1:15000` in the harness environment.

To install just the skill without prompts, run:

```sh
backlog install-skill
```

`backlog install-skill` also accepts `-dir`.

For upgrades from `report-harness-progress`, install `backlog`, carry over any customizations, and retire the old skill directory to avoid loading both skills.

## Upgrade

Run `brew upgrade simonski/backlog/backlog`. A server started through the Homebrew executable link restarts automatically, and open dashboards reload.
