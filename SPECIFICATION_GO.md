# Go application specification

Copy this file into the root of a Go project that ships a command-line client and a server. Set the executable name and default server port for the project.

```sh
TOOLNAME='the name'
DEFAULT_PORT=N
```

Set `TOOLNAME` to the executable name and `DEFAULT_PORT` to the numeric port the server listens on by default. Use these values in the build, CLI, and deployment configuration. Document the module path, executable name, default listen address and port, configuration and data locations, and supported platforms in `README.md`.

Set the hostnames for the deployment profiles in use. Enter the hostname alone, without a scheme or port.

```sh
TAILNET_HOST='mytool.example-tailnet.ts.net'
EXEDEV_HOST='my-vm.exe.xyz'
LOCAL_HOST='localhost'
```

- `TAILNET_HOST` is the full Tailscale HTTPS hostname for the Mac mini’s service. Use the service’s assigned hostname when it has its own Tailscale Services name.
- `EXEDEV_HOST` is the HTTPS hostname assigned to the exe.dev VM, typically `<vm-name>.exe.xyz`, or its configured custom domain. Use the VM name itself when a deployment command asks for `<vm-name>`.
- `LOCAL_HOST` is the browser hostname for a server running on the same computer. Use `localhost` for the loopback profile.

## Module and executable

- Keep one Go module at the repository root. Set its canonical import path and minimum Go version in `go.mod` and commit `go.sum`.
- Use the latest stable Go release for development and builds. Keep libraries on their latest stable releases, including security and patch updates. Review compatibility and run the build and tests when updating the Go version or dependencies. Record any temporary version pin and its reason.
- Build and distribute one executable per supported platform. Use subcommands to select the CLI client, server, or native menu bar client. A client-only installation uses the same executable as a server installation.
- Keep command handling, server behavior, and persistence in separate packages under `internal/`. Keep the executable entry point small.

## Local build and development

Provide a root `Makefile` with these targets.

| Command | Behavior |
| --- | --- |
| `make` or `make help` | Print the available targets and their usage. |
| `make clean` | Remove build artifacts. Preserve configuration and application data. |
| `make build` | Build the executable as `bin/$TOOLNAME`. |
| `make install` | Build and install the executable into Go’s install directory. Use `GOBIN` when set or `bin` under the first `GOPATH` entry. Allow `BINDIR` to override the location. |
| `make run` | Build and run `serve` in the foreground on `DEFAULT_PORT`. |
| `make run -- -p 16000` | Build and run `serve -p 16000`. |

Keep build, test, and deployment automation in the local project harness. Use `go build ./...` to check compilation, `go test ./...` to run tests, and `go fmt ./...` to format Go source. Keep reusable Go or Python tools under `tools/` and record their invocation durations and outcomes in `tools-calls.csv`.

## Homebrew distribution

Use the `github.com/simonski/<reponame>/<the-tool-name>` naming convention for Homebrew distribution. Host the tap in `github.com/simonski/<reponame>` and name the formula after `TOOLNAME`. Homebrew uses `simonski/<reponame>/<the-tool-name>` as the formula name in commands.

```sh
REPONAME='your-repo'
brew tap "simonski/$REPONAME" "https://github.com/simonski/$REPONAME.git"
brew install "simonski/$REPONAME/$TOOLNAME"
brew upgrade "simonski/$REPONAME/$TOOLNAME"
```

Pin stable installs to a tagged release and its source revision. Build the same executable as `make build`, and add a `brew test` smoke check for the installed binary. If the formula offers a development build, expose it through `--HEAD`. Document the tap, install, and upgrade commands. Advance the stable pin after tagging a release.

## Standard commands

| Command | Behavior |
| --- | --- |
| `"$TOOLNAME" setup` | Show the current setup, then prompt for client or server configuration. |
| `"$TOOLNAME" initdb` | For database-backed projects, create the database if needed and apply pending schema migrations. Use the same database configuration and migration code as `serve`. Repeated runs preserve existing data. Exit successfully once the schema is ready. |
| `"$TOOLNAME" serve [-p N]` | Run the server in the foreground. Use `DEFAULT_PORT` unless `-p N` is supplied. |
| `"$TOOLNAME" serve -d [-p N]` | Start the server in the background and return to the prompt once startup succeeds. Use `DEFAULT_PORT` unless `-p N` is supplied. |
| `"$TOOLNAME" stop` | Gracefully stop the running server, whether it was started in the foreground or background. Run this command on the server host. |
| `"$TOOLNAME" setup -start` | Install and start the background server service where supported. Reuse the existing registration when starting a stopped service. |
| `"$TOOLNAME" menu` | Install or reuse the native macOS menu bar registration, start the client in the current user’s GUI session, and return to the prompt. Provide `"$TOOLNAME" menu -foreground` for debugging. |
| `"$TOOLNAME" version` | Show the installed binary’s client, network protocol, and bundled server versions. |
| `"$TOOLNAME" status` | Report the resolved endpoint, the running server’s advertised application and network protocol versions, and background service state where supported. |

Use single-hyphen flags. `-p` requires a numeric port as the next argument. For background startup, accept both `serve -d -p N` and `serve -p N -d`. Local client commands use `DEFAULT_PORT` unless `-p N` selects another port to contact. For `setup -start`, persist the selected port in the service configuration. Document the bind address and how port overrides interact with configured server URLs.

Print human-readable output by default. Where structured output is supported, emit indented JSON on stdout only when `-json` is passed. Send diagnostics to stderr and return a nonzero exit status on failure.

## Server lifecycle

Allow one running server instance of the tool per host. Both forms of `serve` must fail with a nonzero exit status if an instance is already running, even when the requested port differs. Enforce the same rule when concurrent starts are attempted and when `setup -start` launches the service. Report a port held by another application as a bind error.

Use the configured bind address and storage in both modes. Report startup failures to the invoking terminal, including when starting in the background. Document whether startup applies database migrations or requires `initdb` first.

`stop` locates the local instance even when it was started with a custom port. Require a host-local control channel and reject shutdown requests received over remote connections. Drain in-flight work before exiting. For a supervised background instance, stop it through the supervisor so it stays stopped.

## Setup wizard

Before the first prompt, show the current setup. Include the saved role and `TOOL_URL`, the skill installation status, and, where applicable, the database path and initialization state, local listener, server version, and background service status. Show defaults for settings that have yet to be saved. Inspect the database without creating or changing it during this summary.

Ask whether to configure the client or server. In both roles, prompt for `TOOL_URL`, prefilled with the current value or the local HTTP URL built from `LOCAL_HOST` and `DEFAULT_PORT`. In client setup, this is the server URL to connect to. In server setup, this is the URL clients should use, such as the local, tailnet, or exe.dev URL. The server’s listen address and port are separate settings.

- **Client.** Ask whether to install the bundled skill. If accepted, install it into the user’s agent harness and show the destination. Preserve any customized copy already at that location.
- **Server.** For database-backed projects, ask whether to initialize the database only if it has yet to be initialized. If accepted, use the same initialization routine as `initdb`. If it is already initialized, show that state and skip the question.

Save the chosen configuration and report what changed when setup finishes.

## Installable components

- Bundle the project’s agent harness skill in the executable and offer installation during client setup. Allow the installation destination to be overridden. Have the skill explain when to use the CLI, how to configure it, and which commands to run.
- Where a background server applies, provide an explicit installer that uses the platform’s service manager. On macOS, use a per-user `launchd` LaunchAgent that runs the installed executable with `serve`. Persist its arguments, use an absolute executable path, and write logs to documented locations.
- Document how to start, stop, restart, and inspect the service and its logs. Document which platforms have service registration and how to run the foreground server on other platforms.
- Where a native menu bar client applies, make it independently installable from the same binary. Use the configured server URL, show connection status and the application’s primary information, and provide Settings and Quit actions. Keep one menu bar client per user session. Quit closes the menu bar client while the server continues running.
- Keep CLI and server functionality available in builds without native menu bar support. Document which builds provide `menu`.

## Client, network, and server versions

Keep three independent `MAJOR.MINOR.PATCH` versions in `internal/version/version.go`. Apply semantic versioning to each public contract. [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)

| Version | Contract |
| --- | --- |
| Client | CLI commands and output. A new command increments the client minor version and resets the patch version. |
| Network protocol | Requests, responses, and other wire behavior. Increment the major version for incompatible changes, minor for compatible additions, and patch for compatible fixes. |
| Server | Server API and behavior. A new API operation increments the server minor version and resets the patch version. |

Expose the running server’s application version and network protocol version at a documented version endpoint. For each server-backed invocation, compare the installed client version with the advertised server application version, and separately compare the client’s supported network protocol with the server’s advertised protocol.

- Warn on stderr about major or minor differences and include the versions involved. Patch-only differences stay quiet. Keep JSON stdout valid.
- When the client is older, give the documented upgrade command or release location. When the server is older, identify the server as the component to upgrade. A compatible command can still run after the warning. If there is no mutually supported wire protocol, fail before running the requested operation and explain how to restore compatibility.
- If a server-backed command fails and the server version could not be retrieved, report that it is unavailable.
- If the version endpoint is unavailable, document whether clients attempt the operation for compatibility with older servers. Reject a known incompatible protocol in either case.

## Server deployment profiles

| Profile | Run the server | Connect |
| --- | --- | --- |
| Mac mini on Tailscale | Install with Homebrew, run `"$TOOLNAME" setup -start -addr "127.0.0.1:$DEFAULT_PORT"` to register the macOS LaunchAgent, and use Tailscale Serve to forward HTTPS to `http://127.0.0.1:${DEFAULT_PORT}/`. | Open `https://${TAILNET_HOST}/` from an authorized device on the tailnet. |
| Foreground on any supported computer | Run `"$TOOLNAME" serve -addr "127.0.0.1:$DEFAULT_PORT"` in the terminal. | Open `http://${LOCAL_HOST}:${DEFAULT_PORT}/`. |
| exe.dev VM | Install the Linux executable, keep application data on the VM’s persistent disk, and run `"$TOOLNAME" serve -addr "127.0.0.1:$DEFAULT_PORT"` under systemd. Point the exe.dev proxy at the local HTTP port with `ssh exe.dev share port <vm-name> "$DEFAULT_PORT"`. | Open `https://${EXEDEV_HOST}/`. The proxy terminates HTTPS and defaults to private access. |

See the platform references for [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), the [exe.dev HTTPS proxy](https://exe.dev/docs/proxy), and [running a Go service on exe.dev](https://exe.dev/deploy/go).

## README

Keep the root `README.md` concise and runnable from a fresh install. Cover four basics.

1. **Install.** Give the Homebrew tap and install commands and a source-build alternative.
2. **Setup.** Show `"$TOOLNAME" setup`. Explain the initial summary, the `TOOL_URL` prompt for both roles, the optional client skill installation, and when server setup offers database initialization. Document `"$TOOLNAME" initdb` for direct database initialization.
3. **Run as server.** Show `"$TOOLNAME" serve`, the `-p N` and `-d` options, and `"$TOOLNAME" stop` from the server host. For each deployment profile, show how to install the background service where applicable, configure the proxy, and verify the resulting URL.
4. **Run as client.** Show how to check connectivity from the CLI and install and start the menu bar client where supported. Document sign-in requirements and how to upgrade the client after a version warning.

Link to detailed service operations and deployment instructions where needed.
