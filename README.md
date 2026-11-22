# Backlog

Backlog is a local dashboard for tracking project work reported by an agentic harness.

## Install

Install the current build with Homebrew on macOS.

```sh
brew tap simonski/backlog https://github.com/simonski/backlog.git
brew install --HEAD simonski/backlog/backlog
```

From a source checkout, run `make install` to install into `~/.local/bin`. See `backlog -help` for commands and [SPECIFICATION.md](SPECIFICATION.md) for behavior.

## Setup (client)

Run `backlog setup`, choose **client**, and enter the dashboard URL and an optional monitoring username. For Tailscale, use `https://backlog.<tailnet>.ts.net`. Shell values for `BACKLOG_URL` and `BACKLOG_USERNAME` override the saved settings.

On macOS, run `backlog menu` and click the Backlog icon to see project names, ticket counts, and activity dots in the native menu. Open **Settings…** in that menu to change the URL or username. The dashboard shows self-reported names while clients are monitoring.

Use `backlog project ls` for project IDs and `backlog ls -project N` for a project's tickets. Plain `backlog ls` uses the current checkout if it matches a project and shows every project otherwise.

## Setup (server)

Run `backlog setup` and choose **server** to check the local port, server version, and launchd registration. Follow the printed advice.

- On macOS, `backlog setup -start` installs and starts the launchd service. Elsewhere, run `backlog serve`.
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

3. Approve the host under **Services → backlog → Service hosts**, then open the HTTPS URL from a tailnet device.

To allow archive and project-color controls, run the server on loopback and set `-tailscale-archive-host` to the full service hostname and `-tailscale-archive-user` to the permitted login.
