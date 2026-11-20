# Repository Guidelines

## Project Structure

- `SPECIFICATION.dm` describes the planned single-binary Go application. Its CLI reports task progress and issues to the server, which displays project backlogs on a website. Projects appear as the harness reports them.
- `SDLC.md` defines development and release conventions. `README.md` is the entry point for setup and usage documentation.
- The workspace currently contains planning documents. Keep reusable Go or Python tooling under `tools/` and record tool invocation durations and outcomes in `tools-calls.csv`.

## Build, Test, and Development

Once the Go module exists, run these commands from the project root:

- `go build ./...` checks that all packages compile.
- `go test ./...` runs all package tests. Use `go test ./... -cover` to inspect coverage.
- `go fmt ./...` formats Go source using standard Go indentation and layout.

Keep build, test, and deployment automation in the local project harness. Document the CLI and server startup commands in `README.md` when those entry points land.

## Coding and Testing

Use single-hyphen CLI flags and write project documentation in an objective or imperative voice. Put Go tests alongside their packages in `*_test.go` files and name test functions `TestXxx`. For server-backed CLI changes, check that major or minor version mismatches warn on stderr, patch-only differences stay quiet, and JSON stays valid on stdout. Report an unavailable server version explicitly when reporting an unavailable command.

Maintain separate client and server application versions in `internal/version/version.go`. When adding a command or server API operation, bump the minor version and reset the patch version for each affected component. Follow the protocol version policy independently.

## Commits and Pull Requests

Preserve linear Git history. Use commit subjects that describe the change. In pull requests, summarize user-visible changes and list the checks run. When shipped behavior changes, update `README.md`, relevant design documents, and the implementation plan. Record known differences and unfinished targets.
