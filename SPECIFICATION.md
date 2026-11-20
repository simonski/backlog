# Backlog Specification

## Purpose

Provide a live, local view of the work an agentic harness is considering, doing, or finishing across projects. A single Go binary provides a reporting CLI and serves a browser dashboard. A harness skill tells the harness when to report work during a conversation. The harness keeps its own working context and uses the CLI to publish telemetry.

## Projects and task identity

- The CLI derives project identity from the absolute path to the project root. Inside a Git working tree it resolves the Git root, including from a subdirectory. For a non-Git project, invoke the CLI from the project root. The same root identifies the same project across conversations.
- The first successful report creates the project automatically. Each project has one task list shared by all harness conversations working at that root.
- The harness assigns a stable task ID, unique within the project. Renaming a task or reporting on it from another conversation preserves its identity and recorded progress.

## Reporting work

The harness sends an individual task report when it discovers work or when a task's status, progress, or blocker changes. It occasionally sends a project-wide snapshot of the work it currently knows about, including tentative future work. The harness chooses when it has enough context to send a snapshot.

- A new task needs an ID and a title. It can include a description, a status, a completion percentage, and a blocker note. For an existing task, fields omitted from a report retain their values. Future work can start with a short title and be refined later.
- Task statuses are **To do**, **Doing**, **Blocked**, and **Done**. New tasks default to **To do**. Completion is an optional integer from 0 to 100, and **Done** displays as 100%. A blocker is a status on the affected task, with an optional note. If resolving it warrants separate work, the harness can report a new task.
- An individual report creates or updates only its task. A snapshot creates or updates the tasks it lists and marks every previously known, unfinished task it omits as **No longer needed**. Omitted completed tasks remain **Done**. All omitted tasks stay visible, with their recorded details intact.
- **No longer needed** is a dashboard marker on an unfinished task. A later report that includes the task clears the marker and applies any supplied fields. For example, if the harness reports a previously omitted task as **Doing** at 30%, the dashboard displays that report.
- Snapshots apply to the entire project, including tasks reported by another conversation. The server applies each report in one SQLite transaction. When reports overlap, the server applies them in commit order. Invalid reports leave the stored state unchanged.

## Dashboard

- The homepage lists active and inactive projects and links to each project's single-page task view. Project tabs provide direct navigation between unarchived projects. The homepage sorts unarchived projects by the time of their latest successful report, most recent first. Completed and inactive projects remain available. The header and browser favicon use the same stacked-task mark.
- Project cards show counts for **Blocked**, **To do**, **Doing**, **Done**, and **No longer needed**, in that order. Task boards use the same order, with all five columns on one row at viewport widths of 64rem or more. The task view shows reported progress and blocker notes, and visually de-emphasizes tasks marked **No longer needed**. An open dashboard reflects successful updates within three seconds under normal local operation.
- Active projects and tasks display an activity indicator. Newly reported or changed task cards briefly animate. The dashboard respects the browser's reduced-motion preference and shows the server's semantic version in the bottom-right corner.
- People can archive and restore projects in the browser over loopback. Archived projects are accessible through an Archived view and stay out of the main list and tabs until restored. The server continues accepting reports for archived projects. Task changes come through the reporting CLI.

## Local operation and compatibility

- The server defaults to `0.0.0.0:15000` so remote browsers can view the dashboard directly by IP or full Tailscale hostname. The CLI reports to loopback by default. All project mutations require a loopback connection and a loopback HTTP host. The server and reporting CLI accept `-p N` to override the port. SQLite persists projects, tasks, and archive state across restarts.
- The CLI uses single-hyphen flags. Reporting, version, and direct skill installation commands provide machine-readable JSON on stdout. The setup wizard uses human-readable prompts and results. Diagnostics go to stderr. An unavailable server or rejected report produces a nonzero exit status.
- The setup wizard offers separate choices for installing a macOS user service and installing the reporting skill. The service uses the same server flags and database defaults as foreground operation, starts at login, and restarts when it exits. Setup preserves differing service configurations and customized skill files. On other platforms the wizard offers skill installation and the server runs in the foreground.
- Every server-backed CLI invocation checks the advertised server application version. On a major or minor mismatch, warn on stderr with both versions and identify the older component to upgrade. Patch-only differences produce no warning. If the version is unavailable, report that explicitly when a command is unavailable. Apply the separate client, server, and protocol version rules in `SDLC.md`.
- Build and install targets replace executable files atomically. The Homebrew formula builds a pinned source release. A running server follows its launch path, including Homebrew's executable link, and restarts with the same arguments and port when the executable changes. The browser reloads the full page when it detects the new build, even when the semantic version is unchanged.

## Acceptance checks

1. From a Git root and a subdirectory, report a task with the same ID. The dashboard shows one project and one task with the latest reported title, status, and progress.
2. Report a blocked task with a note. Send a snapshot from another conversation that omits the blocked task and an already completed task, and includes a tentative future task. The blocked task is marked **No longer needed**, the completed task stays **Done**, and the tentative task appears under **To do**. Check that project counts and desktop board columns follow the specified status order.
3. Report the omitted blocked task as **Doing**. The marker clears, and the open project page shows the change within three seconds.
4. Archive a project, report another task, and restart the server. The project remains archived with its tasks intact. Restore it and verify that it returns to the main project list and tabs.
5. Check a CLI/server major or minor mismatch and a patch-only mismatch. Verify that warnings follow the version policy and stdout remains valid JSON.
6. Report a task as **Doing** without a completion percentage. The activity indicator animates and remains visible with reduced motion enabled. Replace the running executable with a different build and verify that the open dashboard reloads at the same URL, preserving data and showing the running server version.
7. Run `backlog setup` on macOS, accept both offers, and check that launchd registers Backlog and the reporting skill is installed. Stop the agent, rerun setup with the same options, and check that it loads again. Check that a foreground server occupying the selected port is preserved and that setup can still install the skill.

## Implementation

- The `backlog` CLI provides `setup`, `serve`, `task`, `snapshot`, `install-skill`, and `version`. Setup generates and loads `~/Library/LaunchAgents/local.backlog.plist` on macOS. Snapshots take a JSON array from a file or stdin. Both skill installation paths copy the embedded reporting skill to the user's skills directory or a directory selected with `-dir`. They preserve existing customizations. The Makefile provides `build`, `install`, and `run` targets. See `README.md` for harness setup and invocation examples.
- The Go server embeds the dashboard assets and refreshes the browser view every 1.5 seconds while connected. The browser renders task fields as text and submits only project archive changes.
- The initial protocol is version 1. Increment the protocol version for changes incompatible with the existing wire contract, independently of the application versions.
- Development proceeds through the reporting contract and transactional SQLite storage, CLI and dashboard integration, then the acceptance checks above. This release targets local telemetry on one machine.
