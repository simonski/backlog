# Backlog

Backlog is a live dashboard for tracking project work reported by an agentic harness.

## Install

Install the stable release with Homebrew on macOS.

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew update
brew install simonski/backlog/backlog
```

To build from source, install the latest stable Go release and run `make install` from the checkout. The binary goes into Go’s install directory, `GOBIN` if set or `bin` under the first `GOPATH` entry. Add that directory to `PATH` to run `backlog` by name. See `backlog -help` for commands and [SPECIFICATION.md](SPECIFICATION.md) for behavior.

See [SPECIFICATION_GO.md](SPECIFICATION_GO.md) for the reusable Go application conventions used by this project.

## Setup

Run `backlog setup` to review the current URLs, skill, database, listener, server version, and background service before choosing a role.

- Choose **client** to save `BACKLOG_URL` and an optional username and install the bundled reporting skill if needed. Use `setup -dir PATH` to choose the skill directory. Setup preserves existing customized skills.
- Choose **server** to save the URL clients should use and optionally initialize a new database. Run `backlog initdb` to initialize or migrate it directly. `serve` also applies migrations on startup. Use `-db PATH` to select a different database.

Backlog stores `backlog-client.json`, `backlog-server.json`, and `telemetry.db` under `harness-engineering` in the user’s config directory. On macOS this is normally `~/Library/Application Support`. On Linux it is `$XDG_CONFIG_HOME` or `~/.config`.

## Run as server

### Mac mini on Tailscale

1. On the Mac mini, run `backlog setup -start -addr 127.0.0.1:15000` to install and start the per-user launchd service. Verify it with `backlog status`.
2. In the Tailscale admin console, tag the Mac mini, create `svc:backlog` with endpoint `tcp:443`, and grant access to the intended tailnet clients. [Tailscale Services](https://tailscale.com/docs/features/tailscale-services)
3. On the Mac mini, run the proxy command.

   ```sh
   tailscale serve --service=svc:backlog --https=443 http://127.0.0.1:15000
   ```

4. If approval is pending, approve the host under **Services → backlog → Service hosts**. From an authorized device, open `https://backlog.<tailnet>.ts.net/`.

Run `backlog stop` as the server user on the Mac mini to stop the service. Use `backlog restart` to print the launchd restart command. After `brew upgrade simonski/backlog/backlog`, run `backlog upgrade` to update and verify the running service. Logs are in `~/Library/Logs/backlog.log` and `~/Library/Logs/backlog-access.log`. The LaunchAgent runs in the user’s macOS login session.

Anyone who can open the dashboard can archive projects and tickets. The archive buttons are on each project card, the project title, and the **Blocked**, **To do**, **Done**, and **Rejected** column headings. Use the right-click menu to archive an individual ticket.

To allow project restoration and color changes over Tailscale, keep the loopback bind and add `-tailscale-archive-host` and `-tailscale-archive-user` to the service’s `backlog serve` arguments. Use the full service hostname and the permitted Tailscale login. [Backlog specification](SPECIFICATION.md)

### Local foreground or background

Run `backlog serve -addr 127.0.0.1:15000` in the foreground and open [http://localhost:15000/](http://localhost:15000/). Plain `serve` binds to `0.0.0.0:15000`. Use `serve -d` to detach and return after startup. With a custom port, both `serve -d -p 16000` and `serve -p 16000 -d` work. Backlog allows one running server per host across foreground and background starts.

Use `backlog status` to find the local server, including one on a custom port. Run `backlog stop` on the server host as the server user. Scripts can still use `backlog serve -stop`. On Linux, detached server logs are under `harness-engineering` in the user’s config directory.

For servers from releases before `0.16.0`, use `backlog serve -stop -p N` to stop the legacy instance before changing the listen port.

### exe.dev over HTTPS (planned)

The exe.dev deployment profile is planned. Support for the VM’s HTTPS hostname and service setup remains pending. [exe.dev Go deployment guide](https://exe.dev/deploy/go)

## Run as client

Run `backlog setup`, choose **client**, and save the URL from the server profile. Run `backlog project ls` to check CLI access. On macOS builds with cgo, run `backlog menu` to start the native menu bar client. Use **Settings…** to edit the saved URL or username. An exported `BACKLOG_URL` takes precedence.

If Backlog warns that the client is older than the server, run `brew upgrade simonski/backlog/backlog` or rerun `make install` from an updated source checkout. Run `backlog version` to check the installed build.

`backlog version` shows the CLI, bundled server, and network protocol versions. `backlog status` shows the running server’s versions. Minor protocol differences produce a warning, and incompatible protocol major versions prevent the operation. The original integer protocol `1` is compatible with protocol `1.0.0`.

To report work, choose skill installation during client setup on the harness host and run `backlog project init` from the project checkout. Ticket reporting uses a local Backlog server on the harness host.
