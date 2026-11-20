# Backlog

Backlog displays the work an agentic harness is considering, doing, or finishing. Harnesses send reports through a CLI. The local dashboard groups tasks by project and refreshes as reports arrive.

## Install with Homebrew

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install simonski/backlog/backlog
backlog serve
```

## Build from source

With Go 1.25 or newer and GNU Make, run these commands from this repository.

```sh
make                     # List targets
make build               # Build bin/backlog
make run                 # Build and start the server
make run -- -p 15001     # Use a different port
```

Backlog listens on `0.0.0.0:15000` by default. On the server, open [127.0.0.1:15000](http://127.0.0.1:15000). From another tailnet device, open `http://<server-tailscale-ip>:15000` or use the server's full Tailscale hostname. The dashboard is readable from any network that can reach the listener. Task reports and archive or restore actions require the server's loopback address.

`make run PORT=15001` and `bin/backlog serve -p 15001` select another port. Use `bin/backlog serve -addr 127.0.0.1:15000` to listen only on loopback. The reporting CLI defaults to `127.0.0.1:15000`. `HARNESS_ADDR` can override either command's default address, and `-p` overrides the port.

The database defaults to `harness-engineering/telemetry.db` under the operating system's user configuration directory. Run `bin/backlog serve -db /path/to/telemetry.db` to choose another file.

## Set up a harness

1. Install `backlog` with Homebrew or run `make install` from this repository. The Makefile installs into `~/.local/bin` by default. Use `make install BINDIR=/path/to/bin` to choose another destination. Include the install directory in the environment that starts the harness, for example `export PATH="$HOME/.local/bin:$PATH"` for the default Makefile install.
2. Start the server with `make run` or `backlog serve`. If it runs on another port, set `HARNESS_ADDR=127.0.0.1:15001` in the harness environment or pass `-p 15001` on each report.
3. Run `backlog install-skill` to install the [embedded reporting skill](.agents/skills/report-harness-progress/SKILL.md) under `~/.agents/skills`. For a project-only install, run `backlog install-skill -dir .agents/skills` from that project. Use `-dir` to choose a different skills directory if needed. The command prints the installed path and preserves any customized copy. Ask the harness to use `report-harness-progress` for the work.

## Report progress

Invoke the CLI from the project being reported. Git projects can report from any subdirectory. For other projects, invoke it from the project root.

```sh
backlog task -id sign-in -title "Add sign-in" -status doing -progress 60
backlog task -id sign-in -status blocked -blocker "Waiting for test credentials"
backlog task -id sign-in -status done

backlog snapshot -file - <<'JSON'
[
  {"id":"sign-in"},
  {"id":"account-recovery","title":"Explore account recovery"}
]
JSON
```

Reuse the task ID across reports and conversations. A new task needs a title. Status values are `todo`, `doing`, `blocked`, and `done`. Omitted fields retain their stored values. An empty description or blocker clears its text. A snapshot applies to the whole project. It retains omitted completed tasks and marks other omitted tasks **No longer needed**. An empty array marks all unfinished tasks as no longer needed.

Successful reports print a JSON receipt such as `{"project_id":1,"reported":2}`. Errors and version warnings go to stderr. Run `backlog version` to see the binary's embedded CLI, server, and protocol versions.

## Browse the dashboard

Use the homepage to find recent projects or open a project tab. Project pages show task status, completion, and blockers. Active tasks show an activity indicator even when no percentage has been reported, and newly updated task cards briefly animate. The dashboard respects the browser's reduced-motion setting. Archive or restore a project from its detail page. Archived projects remain accessible from **Archived** and continue to receive reports.

## Upgrade a running server

The bottom-right badge shows the running server's semantic version. Run `brew upgrade simonski/backlog/backlog` for a Homebrew installation. Launch `backlog serve` through the Homebrew link on `PATH` so it can follow the link when Homebrew upgrades it. Use `make build` to upgrade a server started with `make run`, or `make install` to upgrade a server installed with Make. When the executable changes, the server restarts with the same settings and open dashboards reload. Restart a server from an older release once to enable automatic upgrades.

## Verify changes

Run `make build` and `go test ./...`. The [specification](SPECIFICATION.dm) lists the acceptance scenarios.

With Python Playwright and Google Chrome installed, run `python3 tools/check_browser.py bin/backlog` to check live updates, motion, report reconciliation, and archive and restore across a restart. Add `-upgrade-binary /path/to/a/different/backlog` to verify a live upgrade. The check uses a temporary SQLite database and records the result in `tools-calls.csv`.
