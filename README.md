# Backlog

Backlog is a live dashboard for tracking project work reported by an agentic harness.

## Install

Install the stable release with Homebrew on macOS.

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew update
brew install simonski/backlog/backlog
```

From a source checkout, run `make install` to install into `~/.local/bin`. See `backlog -help` for commands and [SPECIFICATION.md](SPECIFICATION.md) for behavior.

## Setup (client)

Replace `<tailnet>` with the part of your tailnet DNS name before `.ts.net`, or use your Backlog server URL.

```sh
export BACKLOG_URL='https://backlog.<tailnet>.ts.net'
backlog setup
```

1. Choose **client**, press Enter to accept `BACKLOG_URL`, and optionally enter a monitoring username.
2. On macOS, optionally run `backlog menu` to start the menu bar client. **Settings…** edits the saved URL and username. An exported `BACKLOG_URL` takes precedence at startup.
3. Run `backlog install-skill` wherever your agent harness runs. Reporting requires a local Backlog server on that host.
4. Run `backlog project ls` to verify that the CLI can reach the server. Then open the value of `BACKLOG_URL` in your browser.

## Setup (server)

Run `backlog setup` and choose **server** to check the local port, server version, and launchd registration. Follow the printed advice.

- On macOS, `backlog setup -start` installs and starts the launchd service. Elsewhere, run `backlog serve`.
- After installing a newer Backlog binary, run `backlog upgrade` on the macOS server host to update and verify the running launchd service. Use `-p N` if the server listens on a different port.
- Open [http://127.0.0.1:15000/](http://127.0.0.1:15000/) on the host. Use `backlog status` to check it or `backlog restart` to print the launchd command.
- Run `backlog install-skill` where the harness reports work. From a project's checkout, run `backlog project init` to register its GitHub repository or a path such as `~/code/example`. Ticket reporting uses the local server.

On macOS, follow the service logs with:

```sh
tail -F ~/Library/Logs/backlog.log ~/Library/Logs/backlog-access.log
```

## Setup (Tailscale)

Publish Backlog as `https://backlog.<tailnet>.ts.net` through a Tailscale Service. HTTPS terminates on port `443` and forwards to the local Backlog port `15000`.

1. In the Tailscale admin console, apply a tag to the Backlog host and create `svc:backlog` with endpoint `tcp:443`. Grant access to the intended tailnet clients. [Tailscale Services](https://tailscale.com/docs/features/tailscale-services)
2. On the tagged host, run:

   ```sh
   tailscale serve --service=svc:backlog --https=443 http://127.0.0.1:15000
   ```

3. If approval is pending, approve the host under **Services → backlog → Service hosts**. Open the HTTPS URL from a tailnet device.

To allow archive and project-color controls over Tailscale, bind `backlog serve` to `127.0.0.1:15000` with `-addr`, set `-tailscale-archive-host` to the full service hostname, and set `-tailscale-archive-user` to the permitted login.
