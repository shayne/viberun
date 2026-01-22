<p align="center">
  <img src="assets/viberun-logo.png" alt="viberun logo" width="320">
</p>

# viberun

`viberun` is an interactive shell for running agent apps. Run `viberun`, then `run <app>` to drop into an agent session inside a persistent Ubuntu container on a remote host. App data is stored under `/home/viberun` and survives container restarts or image updates.

## Quick start (end-to-end)

You need a local machine with `ssh` and a reachable Ubuntu host (VM or server) that you can SSH into with sudo access.

### 1) Install the CLI

```bash
curl -fsSL https://viberun.sh | bash
```

Verify:

```bash
viberun --help
```

Optional overrides (advanced):

```bash
curl -fsSL https://viberun.sh | bash -s -- --nightly
curl -fsSL https://viberun.sh | bash -s -- --dir ~/.local/bin --bin viberun
```

Or with env vars:

```bash
curl -fsSL https://viberun.sh | VIBERUN_INSTALL_DIR=~/.local/bin VIBERUN_INSTALL_BIN=viberun bash
```

### 2) Set up a host (once per VM)

Use any SSH host alias (for example, `myhost` in `~/.ssh/config`):

```bash
viberun setup myhost
```

Optional: in the shell, set a default host (and agent) so you can omit `@host` later:

```bash
config set host myhost
config set agent codex
```

### 3) Start your first app

Open the shell:

```bash
viberun
```

Then run your app inside the shell:

```bash
run hello-world
```

If this is the first run, the server will prompt to create the container. Press Enter to accept.

Detach without stopping the agent: Ctrl-\\ . Reattach later by running `viberun`, then `run hello-world`.
Paste clipboard images into the session with Ctrl-V; `viberun` uploads the image and inserts a `/tmp/viberun-clip-*.png` path.

### 4) Hello-world prompt (paste inside the session)

```
Create a beautiful hello-world web app with a simple, tasteful landing page.
```

### 5) Open the app

While the session is active, `viberun` starts a localhost proxy to the host port. The agent will print a URL like:

```
http://localhost:8080
```

Open it in your browser.
If you've configured app URLs, run `app <name>` then `url` in the shell to see the HTTPS address.

## Common commands

CLI:

```bash
viberun
viberun setup [<host>]
viberun wipe
```

Shell:

```bash
run myapp
rm myapp
apps
app myapp
shell myapp
snapshot
snapshots
restore latest
url
users
delete
```

<details>
<summary>Table of contents</summary>

- [Quick start (end-to-end)](#quick-start-end-to-end)
- [Common commands](#common-commands)
- [Git setup](#git-setup)
- [Development](#development)
- [Architecture](#architecture)
  - [High-level flow](#high-level-flow)
  - [Core components](#core-components)
  - [Session lifecycle](#session-lifecycle)
  - [Setup pipeline](#setup-pipeline)
  - [Networking and ports](#networking-and-ports)
  - [App URLs and proxy](#app-urls-and-proxy)
  - [Snapshots and restore](#snapshots-and-restore)
  - [Host RPC bridge](#host-rpc-bridge)
  - [Configuration and state](#configuration-and-state)
  - [Agents](#agents)
  - [Security model](#security-model)
  - [Wipe (safety)](#wipe-safety)
  - [Repository layout](#repository-layout)
  - [Troubleshooting](#troubleshooting)

</details>

## Git setup

Git, SSH, and the GitHub CLI are installed in containers. viberun seeds `git config --global user.name` and `user.email` from your local Git config into a host-managed config file that is mounted into each app container and applied on startup. This removes the common "first commit" setup step without auto-authing you.

Choose one of these auth paths:

- **SSH**: Make sure your SSH keys are available inside the container, then run `ssh -T git@github.com` to verify access.
- **HTTPS (GitHub CLI)**: Run `gh auth login` and choose HTTPS, then `gh auth setup-git`. Verify with `gh auth status`.

If you update your local Git identity later, restart the app container (or run `app <name>`, then `update`) to re-apply the new values on startup.

## Development

This repo is Go-first and uses `mise` for tool and task orchestration.

### Setup

```bash
mise install
```

### Build

```bash
mise exec -- go build ./cmd/viberun
mise exec -- go build ./cmd/viberun-server
```

### Run locally

```bash
mise exec -- go run ./cmd/viberun -- --help
mise exec -- go run ./cmd/viberun-server -- --help
```

### Test and vet

```bash
mise exec -- go test ./...
mise exec -- go vet ./...
```

### Container image

```bash
mise run build:image
# fallback: docker build -t viberun .
# proxy image (Caddy + auth):
docker build -f Dockerfile.proxy -t viberun-proxy .
```

### E2E and integration

```bash
bin/viberun-e2e-local
bin/viberun-integration
```

### Setup in development

When you run `viberun` via `go run` (or a dev build), setup defaults to staging the local server binary and building the container image locally. This is equivalent to:

```bash
viberun setup --local --local-image myhost
```

Under the hood, it builds a `viberun:dev` image for the host architecture, streams it over `ssh` with `docker save | docker load`, and tags it as `viberun:latest` on the host.

If you want to explicitly pass a local server binary:

```bash
mise exec -- go build -o /tmp/viberun-server ./cmd/viberun-server
viberun setup --local-path /tmp/viberun-server myhost
```

For the full build/test/E2E flow, see `DEVELOPMENT.md`.

## Architecture

### High-level flow

```
viberun (shell)
  -> run <app>
    -> ssh <host>
      -> viberun-server gateway (mux)
        -> viberun-server <app>
          -> docker container viberun-<app>
            -> agent session (tmux)

container port 8080
  -> host port (assigned per app)
    -> mux forward -> http://localhost:<port>
    -> (optional) host proxy (Caddy)
      -> https://<app>.<domain>
```

### Core components

- Client: `viberun` shell on your machine.
- Server: `viberun-server gateway` executed on the host via SSH (no long-running daemon required).
- Container: `viberun-<app>` Docker container built from the `viberun:latest` image.
- Agent: runs inside the container in a tmux session (default provider: `codex`).
- Host RPC: local Unix socket used by the container to request snapshot/restore operations.
- Proxy (optional): `viberun-proxy` (Caddy + `viberun-auth`) for app URLs and login.

### Session lifecycle

1. In the shell, `run <app>` resolves the host (from `@host` or your default config) and starts the `viberun-server gateway` over SSH.
2. The server creates the container if needed, or starts it if it already exists.
3. The agent process is attached via `docker exec` inside a tmux session so it persists across disconnects.
4. `viberun` sets up a local mux forward so you can open the app on `http://localhost:<port>`.

### Setup pipeline

The setup script (run on the host) does the following:

- Verifies the host is Ubuntu.
- Installs Docker (if missing) and enables it.
- Installs Btrfs tools (`btrfs-progs`) for volume snapshots.
- Pulls the `viberun` container image from GHCR (unless using local image mode).
- Downloads and installs the `viberun-server` binary.

If setup is run from a TTY, it will offer to set up a public domain name (same as `proxy setup` in the shell).

Useful setup overrides:

- `VIBERUN_SERVER_REPO`: GitHub repo for releases (default `shayne/viberun`).
- `VIBERUN_SERVER_VERSION`: release tag or `latest`.
- `VIBERUN_IMAGE`: container image override.
- `VIBERUN_PROXY_IMAGE`: proxy container image override (for app URLs).
- `VIBERUN_SERVER_INSTALL_DIR`: install directory on the host.
- `VIBERUN_SERVER_BIN`: server binary name on the host.
- `VIBERUN_SERVER_LOCAL_PATH`: use a local server binary staged over SSH.
- `VIBERUN_SKIP_IMAGE_PULL`: skip pulling from GHCR (used for local builds).

### Networking and ports

- Each app container exposes port `8080` internally.
- The host port is assigned per app (starting at `8080`) and stored in the host server state.
- `viberun` opens an SSH local forward so `http://localhost:<port>` connects to the host port.
- If the proxy is configured, apps can also be served over HTTPS at `https://<app>.<domain>` (or a custom domain). Access requires login by default and can be made public per app.

### App URLs and proxy

`viberun` can optionally expose apps via a host-side proxy (Caddy + `viberun-auth`).
Set it up once per host from the shell:

```bash
proxy setup [<host>]
```

You'll be prompted for a base domain and public IP (for DNS), plus a primary username/password.
Create an A record (or wildcard) pointing to the host's public IP.

After setup (in the shell):

- `app <name>` then `url` shows the current URL and access status.
- `url public` or `url private` toggles access (default requires login).
- `url set-domain <domain>` or `url reset-domain` manages custom domains.
- `url disable` or `url enable` turns the URL off/on.
- `url open` opens the URL in your browser.
- `users list`/`users add`/`users remove`/`users set-password` manages login accounts; `users` (inside app context) controls who can access the app.

If URL settings change, run `update` inside the app context to refresh `VIBERUN_PUBLIC_URL` and `VIBERUN_PUBLIC_DOMAIN` inside the container.

### Snapshots and restore

Snapshots are Btrfs subvolume snapshots of the app's `/home/viberun` volume (auto-incremented versions).
On the host, each app uses a loop-backed Btrfs file under `/var/lib/viberun/apps/<app>/home.btrfs`.

- `snapshot` creates the next `vN` snapshot (inside app context).
- `snapshots` lists versions with timestamps (inside app context).
- `restore <vN|latest>` restores from a snapshot (`latest` picks the highest `vN`) (inside app context).
- `rm <app>` (global) or `delete` (inside app context) removes the container, the app volume + snapshots, and the host RPC directory.

Restore details:
- The host stops the container (if running) to safely unmount the volume.
- The current `@home` subvolume is replaced by a new writable snapshot of `@snapshots/vN`.
- The container is started again, and s6 reloads services from `/home/viberun/.local/services`.

### Host RPC bridge

When you open a session, the server creates a Unix socket on the host and mounts it into the container at `/var/run/viberun-hostrpc`. The container uses it to request snapshot and restore operations. Access is protected by a per-session token file mounted alongside the socket.

### Configuration and state

Local config lives at `~/.config/viberun/config.toml` (or `$XDG_CONFIG_HOME/viberun/config.toml`) and stores:

- `default_host`
- `agent_provider`
- `hosts` (alias mapping)

Host server state lives at `~/.config/viberun/server-state.json` (or `$XDG_CONFIG_HOME/viberun/server-state.json`) and stores the port mapping for each app.

Proxy config (when enabled) lives at `/var/lib/viberun/proxy.toml` (or `$VIBERUN_PROXY_CONFIG_PATH`) and stores the base domain, access rules, and users.
When enabled, the server injects `VIBERUN_PUBLIC_URL` and `VIBERUN_PUBLIC_DOMAIN` into containers.

### Agents

Supported agent providers:

- `codex` (default)
- `claude`
- `gemini`
- `ampcode` (alias: `amp`)
- `opencode`

Custom agents can be run via `npx:<pkg>` or `uvx:<pkg>` (for example, `config set agent npx:@sourcegraph/amp@latest` in the shell).

Set the default agent in the shell with `config set agent <provider>`.

Base skills are shipped in `/opt/viberun/skills` and symlinked into each agent's skills directory. User skills can be added directly to the agent-specific skills directory under `/home/viberun`.

### Security model

- All control traffic goes over the mux over SSH; the server is invoked on demand and does not expose a network port.
- The host RPC socket is local-only and protected by filesystem permissions and a per-session token.
- Containers are isolated by Docker and only the app port is exposed.
- App URLs are optional: the proxy requires login by default and can be made public per app by running `app <app>` then `url public` in the shell.

### Wipe (safety)

`viberun wipe [<host>]` deletes local config and wipes all viberun data on the host.
It requires a TTY and asks for a yes/no confirmation, then asks you to type `WIPE`.

On the host, wipe removes:

- All containers named `viberun-*`, any containers using `viberun` images, and the proxy container (default `viberun-proxy`).
- All `viberun` images (including the proxy image).
- App data and snapshots under `/var/lib/viberun` (including per-app Btrfs volumes).
- Host RPC sockets in `/tmp/viberun-hostrpc` and `/var/run/viberun-hostrpc`.
- `/etc/viberun`, `/etc/sudoers.d/viberun-server`, and `/usr/local/bin/viberun-server`.

Locally, it removes `~/.config/viberun/config.toml` (and legacy config if present).

### Repository layout

- `cmd/`: Go entrypoints (`viberun`, `viberun-server`, `viberun-auth`).
- `internal/`: Core packages (config, server state, SSH args, target parsing, TUI helpers).
- `bin/`: Helper scripts for installs, integration/E2E flows, and container utilities.
- `skills/`: Codex skill definitions used inside containers.
- `config/`: Shell/TMUX/Starship config, auth assets, and runtime configs.
- `Dockerfile`: Base container image definition.
- `Dockerfile.proxy`: Proxy image definition (Caddy + auth).

### Troubleshooting

- `unsupported OS: ... expected ubuntu`: setup currently supports Ubuntu only.
- `docker is required but was not found in PATH`: install Docker on the host or re-run setup.
- `missing btrfs on host`: rerun setup to install `btrfs-progs` and ensure sudo access.
- `no host provided and no default host configured`: run `viberun`, then `config set host myhost` or use `run myapp@host`.
- `container image architecture mismatch`: delete and recreate the app (`rm myapp` in the shell).
- `proxy is not configured`: run `proxy setup` (then `app myapp`, `url`).
