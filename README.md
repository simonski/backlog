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

Run `backlog setup`. Choose **server** to review the local port, server version, and service registration. Choose **client** to save the server URL and an optional username. Backlog initializes SQLite automatically when the server starts.

## Run as server

### Mac mini on Tailscale

1. On the Mac mini, run `backlog setup -start -addr 127.0.0.1:15000` to install and start the per-user launchd service. Verify it with `backlog status`.
2. In the Tailscale admin console, tag the Mac mini, create `svc:backlog` with endpoint `tcp:443`, and grant access to the intended tailnet clients. [Tailscale Services](https://tailscale.com/docs/features/tailscale-services)
3. On the Mac mini, run the proxy command.

   ```sh
   tailscale serve --service=svc:backlog --https=443 http://127.0.0.1:15000
   ```

4. If approval is pending, approve the host under **Services → backlog → Service hosts**. From an authorized device, open `https://backlog.<tailnet>.ts.net/`.

Use `backlog restart` to print the launchd restart command. After `brew upgrade simonski/backlog/backlog`, run `backlog upgrade` to update and verify the running service. Logs are in `~/Library/Logs/backlog.log` and `~/Library/Logs/backlog-access.log`.

To enable archive and project-color controls over Tailscale, keep the loopback bind and add `-tailscale-archive-host` and `-tailscale-archive-user` to the service’s `backlog serve` arguments. Use the full service hostname and the permitted Tailscale login. [Backlog specification](SPECIFICATION.md)

### Local foreground

Run `backlog serve -addr 127.0.0.1:15000` on any supported computer and open [http://localhost:15000/](http://localhost:15000/). Keep the terminal open while the server runs. Use `-p N` on `serve` and `status` to select a different port.

### exe.dev over HTTPS

1. On the VM, install Backlog from a source checkout with `make install`. Configure a systemd service to run `backlog serve -addr 127.0.0.1:15000` as the installing user. Use the absolute executable path printed by `make install` in `ExecStart` and enable the service to start at boot. [exe.dev Go deployment guide](https://exe.dev/deploy/go)
2. From your computer, run `ssh exe.dev share port <vm-name> 15000`, replacing `<vm-name>` with the VM name. Open `https://<vm-name>.exe.xyz/` and sign in to exe.dev. The HTTPS proxy is private by default. [exe.dev proxy documentation](https://exe.dev/docs/proxy)

## Run as client

Run `backlog setup`, choose **client**, and save the URL from the server profile. Run `backlog project ls` to check CLI access or `backlog menu` to start the macOS menu bar client. Use **Settings…** to edit the saved URL or username. An exported `BACKLOG_URL` takes precedence. For a private exe.dev deployment, use the browser dashboard to complete exe.dev sign-in.

If Backlog warns that the client is older than the server, run `brew upgrade simonski/backlog/backlog` or rerun `make install` from an updated source checkout. Run `backlog version` to check the installed build.

`backlog version` shows the CLI, bundled server, and network protocol versions. `backlog status` shows the running server’s versions. Minor protocol differences produce a warning, and incompatible protocol major versions prevent the operation. The original integer protocol `1` is compatible with protocol `1.0.0`.

To report work, run `backlog install-skill` on the harness host and `backlog project init` from the project checkout. Ticket reporting uses a local Backlog server on the harness host.
