# Backlog Specification

## Purpose

Provide a live, local view of the work an agentic harness is considering, doing, or finishing across projects. A single Go binary provides a reporting CLI and serves a browser dashboard. A harness skill tells the harness when to report work during a conversation. The harness keeps its own working context and uses the CLI to publish telemetry.

## Projects and task identity

- The CLI derives project identity from the absolute path to the project root. Inside a Git working tree it resolves the Git root, including from a subdirectory. For a non-Git project, invoke the CLI from the project root. The same root identifies the same project across conversations.
- The first successful report or document upload creates the project automatically. Each project has one task list shared by all harness conversations working at that root.
- The harness assigns a stable task ID, unique within the project. Renaming a task or reporting on it from another conversation preserves its identity and recorded progress.

## Reporting work

The harness sends an individual task report when it discovers work or when a task's status, progress, or blocker changes. It occasionally sends a project-wide snapshot of the work it currently knows about, including tentative future work. The harness chooses when it has enough context to send a snapshot.

- A new task needs an ID and a title. It can include a description, a status, a completion percentage, and a blocker note. For an existing task, fields omitted from a report retain their values. Future work can start with a short title and be refined later.
- Task statuses are **To do**, **Doing**, **Blocked**, and **Done**. New tasks default to **To do**. Completion is an optional integer from 0 to 100, and **Done** displays as 100%. A blocker is a status on the affected task, with an optional note. If resolving it warrants separate work, the harness can report a new task.
- An individual report creates or updates only its task. A snapshot creates or updates the tasks it lists and marks every previously known, unfinished task it omits as **Rejected**. Omitted completed tasks remain **Done**. All omitted tasks stay visible, with their recorded details intact.
- **Rejected** is a dashboard marker on an unfinished task. A later report that includes the task clears the marker and applies any supplied fields. For example, if the harness reports a rejected task as **Doing** at 30%, the dashboard displays that report.
- Snapshots apply to the entire project, including tasks reported by another conversation. The server applies each report in one SQLite transaction. When reports overlap, the server applies them in commit order. Invalid reports leave the stored state unchanged.

## Ticket commands

- `ls` lists the current project's tickets, including completed and rejected tickets. The optional `-id` selects one or more tickets by stable ID, in the order given. A project with no reports has an empty list.
- `update -id` applies the supplied title, description, status, progress, or blocker fields to every selected ticket. `todo`, `doing`, `block`, and `done` set the corresponding status. `block` accepts an optional `-blocker` note. The status commands preserve other fields unless supplied and clear the rejection marker for selected tickets.
- Each command accepts a single ID or a comma-separated list with optional spaces around IDs. Empty and duplicate IDs are errors. A requested ID must exist in the current project. A bulk change runs in one transaction and rolls back all changes if any selected ticket is missing.

## Dashboard

- The homepage lists active and inactive projects and links to each project's task board. Project tabs provide direct navigation between unarchived projects. Projects with visible Doing tickets sort first, followed by other projects. Each group sorts by latest project activity, including ticket status changes, progress reports, snapshots, archives, and document uploads. A document upload promotes its project above the normal order for six seconds. Project cards slide into their new positions when their rank changes. Reduced-motion mode applies the new order immediately. The header and browser favicon use the same stacked-task mark.
- The homepage places the activity grid to the right of **Projects**. It has one row per unarchived project, marked with a pastel square matching its card. Range buttons default to four weeks. One and two weeks are always available. One month, three months, six months, and one year become available when recorded history reaches the range. The shorter ranges use day columns and the longer ranges use week columns. Each occupied cell uses its project's pastel tint and shows one status-colored square per distinct ticket reported or updated in the period, using its latest status in the period. Date ticks are centered above their columns. The grid uses the server's local time zone and grows to fit all pixels without internal scrollbars.
- Project cards show counts for **Blocked**, **To do**, **Doing**, **Done**, and **Rejected**, in that order. The homepage Done count is a lifetime count of distinct tickets recorded as completed, including archived or later reopened tickets. Task-board counts reflect visible tickets. Boards use the same column order and fit all five columns on one row at viewport widths of 64rem or more. The task view shows progress and blocker notes, and visually de-emphasizes rejected tickets. An open dashboard reflects successful updates within three seconds under normal local operation.
- Active projects and tasks display an activity indicator. Newly reported or changed task cards briefly animate. The dashboard respects the browser's reduced-motion preference and shows the server's semantic version in the bottom-right corner.
- A red badge next to a project title shows the blocked-ticket count when it is nonzero. The project board shows live elapsed time in **Doing** and a fixed total in **Done**, formatted as hours, minutes, and seconds. Timing accumulates only during **Doing**. Progress reports and server restarts preserve a running timer, and a later return to **Doing** resumes the accumulated total.
- For users with archive access, project cards and tickets expose **Archive** in their context menus. **Blocked**, **To do**, **Done**, and **Rejected** columns expose **Archive all**. The project title and the **Done** column also provide archive buttons. Every archive action opens a confirmation dialog. Archiving hides the affected content from the board and activity grid, and excludes it from visible status counts. Reports and CLI listings still include archived records.
- At compact widths, tighter header, navigation, and card spacing keeps more of the task dashboard visible. This layout also serves the macOS Quick Backlog window.

## Project documents

- `doc -file PATH [-name NAME]` uploads UTF-8 Markdown to the current project over loopback. For standard input, use `-file -` and supply `-name`. The exact name identifies a document within its project, and uploading it again replaces the content at the same URL.
- The project board links to all of its documents above the ticket columns. The homepage card links directly to a README and specification when documents with standard matching names exist. The document view renders Markdown headings, links, lists, code blocks, and tables as HTML. Raw HTML and dangerous links are omitted. Archived projects hide their documents from the browser.
- `doc -clear` removes uploads for the current project while preserving its tickets and source files.

## Local operation and compatibility

- The server defaults to `0.0.0.0:15000` so remote browsers can view the dashboard directly by IP or full Tailscale hostname. CLI writes require a loopback connection and a loopback HTTP host. Archive actions also support an opt-in Tailscale Serve route with HTTPS, an exact host, an authorized login, and a loopback listener. The server and reporting CLI accept `-p N` to override the port. SQLite persists projects, tasks, and archive state across restarts.
- The CLI uses single-hyphen flags. The ticket, reporting, version, and direct skill installation commands print readable output on stdout by default and indented JSON with `-json`. The setup wizard uses readable prompts and results. Diagnostics go to stderr. An unavailable server or rejected report produces a nonzero exit status.
- The setup wizard offers separate choices for installing a macOS user service and installing the reporting skill. The service uses the same server flags and database defaults as foreground operation, starts at login, and restarts when it exits. Setup preserves differing service configurations and customized skill files. On other platforms the wizard offers skill installation and the server runs in the foreground.
- On macOS, `backlog menu` adds a ticket icon to the menu bar for the duration of the process. **Open Backlog** opens the local dashboard in the default browser. **Quick Backlog** opens one reusable dashboard window. The command accepts the same local address and port options as the reporting CLI.
- Every server-backed CLI invocation checks the advertised server application version. On a major or minor mismatch, warn on stderr with both versions and identify the older component to upgrade. Patch-only differences produce no warning. If the version is unavailable, report that explicitly when a command is unavailable. Apply the separate client, server, and protocol version rules in `SDLC.md`.
- `status` reports the dashboard URL, local port, Backlog identity and version, `backlog serve` process IDs when detected, and launchd registration on macOS. `BACKLOG_URL` sets the displayed URL independently of the local probe address. A refused connection or a listener without a valid Backlog version response produces a failing exit and an explicit unavailable-version diagnostic. Readable and JSON reports both retain the results of the other checks.
- `ps` aliases `status` including its flags. Status output distinguishes the CLI, bundled server, and advertised running server releases, with advice in the report and compatibility warnings on stderr. `restart` prints the macOS command for kicking a registered launchd job, bootstrapping an installed agent, or running initial setup.
- `serve` writes HTTP access records to stdout with the connection peer, method, path, status, bytes, and duration. Access records exclude query strings, headers, and bodies. Startup and error diagnostics use stderr. The macOS LaunchAgent directs stdout to `backlog-access.log` and stderr to `backlog.log` under `~/Library/Logs`.
- When a port is already occupied, `serve` exits before opening SQLite. Start-up diagnostics identify a Backlog instance if the listener answers the version check over loopback and point to `-p` for a different port.
- `serve -stop` confirms the local server's identity, sends `POST /api/stop`, and waits for the listener to close. The server lets in-flight requests drain before exiting. For a known macOS Backlog launchd job that owns the selected listener, the CLI unloads the job so it does not restart automatically.
- Build and install targets replace executable files atomically. The Homebrew formula builds a pinned source release. A running server follows its launch path, including Homebrew's executable link, and restarts with the same arguments and port when the executable changes. The browser reloads the full page when it detects the new build, even when the semantic version is unchanged.

## Acceptance checks

1. From a Git root and a subdirectory, report a task with the same ID. The dashboard shows one project and one task with the latest reported title, status, and progress.
2. Report a blocked task with a note. Send a snapshot from another conversation that omits the blocked task and an already completed task, and includes a tentative future task. The blocked task is marked **Rejected**, the completed task stays **Done**, and the tentative task appears under **To do**. Check that project counts and desktop board columns follow the specified status order.
3. Report the omitted blocked task as **Doing**. The marker clears, and the open project page shows the change within three seconds.
4. Cancel and confirm both archive actions. Check that completed tickets disappear from the board, totals, and activity after archiving **Done**, and that the project disappears from the dashboard after archiving its title. Report another task and restart the server. Verify that the project remains hidden and the CLI still lists its tickets.
5. Check the default text and indented `-json` output of each reporting, version, and direct skill installation command. Check a CLI/server major or minor mismatch and a patch-only mismatch. Verify that warnings follow the version policy and stdout with `-json` remains valid JSON.
6. Report a task as **Doing** without a completion percentage. The activity indicator animates and remains visible with reduced motion enabled. Replace the running executable with a different build and verify that the open dashboard reloads at the same URL, preserving data and showing the running server version.
7. Run `backlog setup` on macOS, accept both offers, and check that launchd registers Backlog and the reporting skill is installed. Stop the agent, rerun setup with the same options, and check that it loads again. Check that a foreground server occupying the selected port is preserved and that setup can still install the skill.
8. Run `backlog menu` in a macOS desktop session. Check that the ticket icon has both menu actions, that both open the configured dashboard, and that Ctrl-C removes the icon while the server keeps running.
9. List project tickets with and without an ID filter. Edit fields and set statuses with single and comma-separated IDs. Include an unknown ID in a bulk change and verify that every selected ticket retains its previous status.
10. Check the four-week default grid in the homepage heading at desktop and mobile widths. Verify that longer presets are enabled as historical data becomes available and that each period includes only the latest status for each ticket.
11. Start a ticket, report progress, block it, and resume it. Check the red blocked-count badge, the advancing timer, and the fixed total once the ticket is done.
12. Run `backlog status` against Backlog, an unrelated listener, and a closed port. Verify the three report outcomes, the machine-readable diagnostics, and the `BACKLOG_URL` display override.

## Implementation

- Setup generates and loads `~/Library/LaunchAgents/local.backlog.plist` on macOS. Snapshots take a JSON array from a file or stdin. Both skill installation paths copy the embedded reporting skill to the user's skills directory or a directory selected with `-dir`. They preserve existing customizations. The Makefile provides `build`, `install`, and `run` targets. See `README.md` for harness setup and invocation examples.
- Version 0.7.0 adds `ls`, `update`, `block`, `todo`, `doing`, and `done`. `GET /api/tasks` lists tickets on loopback using the project root and optional repeated `id` query parameters. `PATCH /api/tasks` accepts a project root and a JSON array of changes in the `tasks` field. Existing reporting clients continue to use `POST /api/reports`. Update the Homebrew source pin after tagging the release.
- CLI version 0.7.1 installs the bundled skill under the name `backlog`. Existing skill directories are preserved during installation.
- Server version 0.7.1 records ticket activity in SQLite in the same transaction as each report or edit. Activity starts with the first successful write after upgrading an existing database.
- Version 0.6.0 adds the macOS menu command and the compact dashboard layout. The menu uses AppKit and WebKit through cgo. It runs separately from the server and starts on demand. Other commands remain available in builds without cgo.
- Version 0.8.0 adds the `doc` command and the server's document upload and viewing routes. It also adds dashboard archive actions and persists ticket timers in the same transaction as status changes. Existing active tickets start timing at upgrade. Previously completed tickets without recorded time display that the time was not recorded.
- Server version 0.9.0 adds individual ticket archiving, extends column archiving to Rejected, and adds homepage document shortcuts and activity presets.
- Server version 0.9.1 can authorize archive controls through a configured Tailscale Serve HTTPS route.
- Server version 0.10.0 adds the homepage project color palette and document clearing. Preset colors have one owner and are reassigned transactionally. CLI version 0.9.0 adds `doc -clear`.
- CLI version 0.10.0 adds `status` using the existing server version endpoint.
- Server version 0.10.1 reports occupied ports explicitly during startup.
- Version 0.11.0 adds `serve -stop` and the loopback-only shutdown API.
- CLI version 0.12.0 adds `ps`, `restart`, and status upgrade advice.
- Server version 0.11.1 adds the access log. CLI version 0.12.1 adds stdout capture to generated LaunchAgents and upgrades an exact match for the older standard configuration.
- The Go server embeds the dashboard assets and refreshes the browser view every 1.5 seconds while connected. The browser renders task fields as text and submits confirmed archive changes.
- The initial protocol is version 1. Increment the protocol version for changes incompatible with the existing wire contract, independently of the application versions.
- Development proceeds through the reporting contract and transactional SQLite storage, CLI and dashboard integration, then the acceptance checks above. Version 0.5.0 introduces the new CLI output modes and the **Rejected** dashboard label. Update the Homebrew source pin after tagging the release. This release targets local telemetry on one machine.
