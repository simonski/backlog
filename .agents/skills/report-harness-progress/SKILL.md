---
name: report-harness-progress
description: Send progress telemetry to the local Backlog dashboard during project work. Use when a project uses this dashboard or the user asks to track work there.
---

# Report harness progress

Run `backlog` from the project being reported. Use the project root for a non-Git project. Keep work context in the harness and use the dashboard as a view for the person following along.

- Assign a stable task ID before the first report. Reuse it for later updates and when resuming known work in another conversation.
- Report new work and meaningful changes as they happen. Include a percentage when there is a reasonable estimate. Put the blocker on the affected task. Report a separate task if resolving the blocker is itself separate work.
- Send an occasional project-wide snapshot when the overall picture is clear enough, including tentative future work. Include known unfinished tasks that should stay visible as current work. Omitted unfinished tasks are marked **Rejected**, even when another conversation reported them.
- If reporting fails, use the CLI diagnostic to explain the telemetry gap and continue the underlying work.

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

The CLI defaults to `127.0.0.1:15000`. Set `HARNESS_ADDR` in the harness environment or pass `-p N` for a server on a different loopback port.
