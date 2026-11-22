---
name: backlog
description: Send progress telemetry to the local Backlog dashboard during project work. Use when a project uses this dashboard or the user asks to track work there.
---

# Backlog

Run `backlog` from the project being reported. Use the project root for a non-Git project. Keep work context in the harness and use the dashboard as a view for the person following along.

Use the `backlog` binary for all Backlog commands. Do not use `curl`, Python, or direct HTTP requests to talk to Backlog.

- Assign a stable task ID before the first report. Reuse it for later updates and when resuming known work in another conversation.
- Use `backlog ls` to recover existing ticket IDs and statuses. It infers a project when the checkout matches. Use `backlog project ls` to find the project ID and `backlog ls -project N` to select it explicitly. Pass `-id ID` or a comma-separated list to inspect selected tickets, and use `-json` for structured output.
- Report new work and meaningful changes as they happen. Include a percentage when there is a reasonable estimate. Put the blocker on the affected task. Report a separate task if resolving the blocker is itself separate work.
- Use `task -id ID -title TITLE` to create a ticket. For existing tickets, use `update` to change fields or `todo`, `doing`, `block`, and `done` to set status. Pass one ID or a comma-separated list to apply the same change to several tickets in one transaction.
- Send an occasional project-wide snapshot when the overall picture is clear enough, including tentative future work. Include known unfinished tasks that should stay visible as current work. Omitted unfinished tasks are marked **Rejected**, even when another conversation reported them.
- If reporting fails, use the CLI diagnostic to explain the telemetry gap and continue the underlying work.

```sh
backlog task -id sign-in -title "Add sign-in" -status doing -progress 60
backlog task -id account-recovery -title "Explore account recovery"
backlog ls -id sign-in,account-recovery -json
backlog todo -id sign-in,account-recovery
backlog doing -id sign-in,account-recovery
backlog update -id sign-in,account-recovery -progress 75
backlog block -id sign-in -blocker "Waiting for test credentials"
backlog done -id sign-in,account-recovery
```

The ticket commands require CLI and server version 0.7.0 or later. On older versions, use `task -id ID` with the desired fields once per ticket.

## Publish project documentation

Run `backlog project init` from the project checkout to register its GitHub repository or local path. When initializing or refreshing a project in Backlog, upload only its root `README.md` and `SPECIFICATION.md` if they exist. If the project uses `SPEC.md` as its main specification, upload it under the name `SPECIFICATION.md`. Do not scan documentation directories or bulk-upload files. Upload any additional documents only on explicit request.

The first upload can create the project. Upload under the same name to replace a document. The dashboard links to these documents from the project card and board and renders them as HTML.

```sh
backlog project init
backlog doc -file README.md
backlog doc -file SPECIFICATION.md
```

`backlog project init` requires CLI 0.13.0 and server 0.12.0 or later. `backlog doc` requires CLI and server version 0.8.0 or later. If `backlog` on PATH is older, use the newer local install at `~/.local/bin/backlog` when available.

## Report a snapshot

```sh
backlog snapshot -file - <<'JSON'
[
  {"id":"sign-in"},
  {"id":"account-recovery","title":"Explore account recovery"}
]
JSON
```

The CLI defaults to `127.0.0.1:15000`. Set `HARNESS_ADDR` in the harness environment or pass `-p N` for a server on a different loopback port.
