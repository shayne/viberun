<p align="center">
  <img src="assets/viberun-logo.png" alt="viberun logo" width="320">
</p>

# viberun

`viberun` is a shell-first, agent-native app host. Run `viberun` to open the shell, then `vibe <app>` to jump into a persistent Ubuntu container on a remote host. App data is stored under `/home/viberun` and survives container restarts or image updates.

## Quick start (end-to-end)

You need a local machine with `ssh` and a reachable Ubuntu host (VM or server) that you can SSH into with sudo access.

### 1) Install the CLI

```bash
curl -fsSL https://viberun.sh | bash
```

Verify:

```bash
viberun --version
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

### 2) Connect a host (once per VM)

Start the shell:

```bash
viberun
```

If this is the first time, you'll see a setup banner. Type:

```bash
setup
```

Enter your SSH login (for example, `user@myhost` in `~/.ssh/config`).
You can also run setup directly from the CLI:

```bash
viberun setup myhost
```

### 3) Start your first app

Inside the shell:

```bash
vibe hello-world
```

If this is the first run, the server will create the container.

Detach without stopping the agent: Ctrl-\\ . Reattach later with `vibe hello-world`.
Paste clipboard images into the session with Ctrl-V; viberun uploads the image and inserts a `/tmp/viberun-clip-*.png` path.

### 4) Hello-world prompt (paste inside the session)

```
Create a beautiful hello-world web app with a simple, tasteful landing page.
```

### 5) Open the app

From the shell:

```bash
open hello-world
```

When the shell starts, viberun forwards app ports automatically. `open` prefers a public URL if configured; otherwise it uses the localhost URL.

## Common commands

Shell commands (inside `viberun`):

```bash
vibe myapp
open myapp
apps
app myapp
rm myapp
```

CLI commands (advanced):

```bash
viberun setup [<host>]
viberun wipe [<host>]
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

- **SSH (agent forwarding)**: Start viberun with `VIBERUN_FORWARD_AGENT=1 viberun`, then `vibe <app>`. For existing apps, run `app <app>` followed by `update` once to recreate the container with the agent socket mounted. Then `ssh -T git@github.com` inside the container to verify access.
- **HTTPS (GitHub CLI)**: Run `gh auth login` and choose HTTPS, then `gh auth setup-git`. Verify with `gh auth status`.

If you update your local Git identity later, restart the app container (or run `app <app>` then `update`) to re-apply the new values on startup.

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

When you run `viberun` via `go run` (or set `VIBERUN_DEV=1`), setup defaults to staging the local server binary and building the container image locally. Run:

```bash
viberun
setup
```

Or directly:

```bash
viberun setup myhost
```

Under the hood, it builds a `viberun:dev` image for the host architecture, streams it over `ssh` with `docker save | docker load`, and tags it as `viberun:latest` on the host.

For the full build/test/E2E flow, see `DEVELOPMENT.md`.

## Architecture

### High-level flow

```
viberun (shell)
  -> vibe <app>
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

- Client: `viberun` CLI on your machine.
- Server: `viberun-server gateway` executed on the host via SSH (no long-running daemon required).
- Container: `viberun-<app>` Docker container built from the `viberun:latest` image.
- Agent: runs inside the container in a tmux session (default provider: `codex`).
- Host RPC: local Unix socket used by the container to request snapshot/restore operations.
- Proxy (optional): `viberun-proxy` (Caddy + `viberun-auth`) for app URLs and login.

### Session lifecycle

1. `viberun` connects to the host (from your saved config) and starts the `viberun-server gateway` over SSH.
2. `vibe <app>` creates the container if needed, or starts it if it already exists.
3. The agent process is attached via `docker exec` inside a tmux session so it persists across disconnects.
4. `viberun` forwards app ports when the shell starts so you can open `http://localhost:<port>` immediately.

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
- `viberun` forwards app ports when the shell starts so `http://localhost:<port>` connects to the host port.
- If the proxy is configured, apps can also be served over HTTPS at `https://<app>.<domain>` (or a custom domain). Access requires login by default and can be made public per app.

### App URLs and proxy

`viberun` can optionally expose apps via a host-side proxy (Caddy + `viberun-auth`).
Set it up once per host (inside the shell):

```bash
proxy setup [<host>]
```

You'll be prompted for a base domain and public IP (for DNS), plus a primary username/password.
Create an A record (or wildcard) pointing to the host's public IP.

After setup, in the shell:

- `app <app>` then `url` shows the current URL and access status.
- `url public` or `url private` toggles access (default requires login).
- `url set-domain <domain>` or `url reset-domain` manages custom domains.
- `url disable` or `url enable` turns the URL off/on.
- `url open` opens the URL in your browser.
- `users` manages login accounts; `app <app>` then `users` controls who can access the app.

If URL settings change, run `app <app>` then `update` to refresh `VIBERUN_PUBLIC_URL` and `VIBERUN_PUBLIC_DOMAIN` inside the container.

### Snapshots and restore

Snapshots are Btrfs subvolume snapshots of the app's `/home/viberun` volume (auto-incremented versions).
On the host, each app uses a loop-backed Btrfs file under `/var/lib/viberun/apps/<app>/home.btrfs`.

- `app <app>` then `snapshot` creates the next `vN` snapshot.
- `app <app>` then `snapshots` lists versions with timestamps.
- `app <app>` then `restore <vN|latest>` restores from a snapshot (`latest` picks the highest `vN`).
- `rm <app>` removes the container, the app volume + snapshots, and the host RPC directory.

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

Custom agents can be run via `npx:<pkg>` or `uvx:<pkg>` (for example, set `config set agent npx:@sourcegraph/amp@latest` in the shell).

Set the default agent with `config set agent <provider>` in the shell.
To forward your local SSH agent into the container, start viberun with `VIBERUN_FORWARD_AGENT=1 viberun`. For existing apps, run `app <app>` then `update` once to recreate the container with the agent socket mounted.

Base skills are shipped in `/opt/viberun/skills` and symlinked into each agent's skills directory. User skills can be added directly to the agent-specific skills directory under `/home/viberun`.

### Security model

- All control traffic goes over the mux over SSH; the server is invoked on demand and does not expose a network port.
- The host RPC socket is local-only and protected by filesystem permissions and a per-session token.
- Containers are isolated by Docker and only the app port is exposed.
- App URLs are optional: the proxy requires login by default and can be made public per app with `app <app>` then `url public`.

### Wipe (safety)

`viberun wipe [<host>]` deletes local config and wipes all viberun data on the host.
It requires a TTY and asks you to type `WIPE`.

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
- `docker is required but was not found in PATH`: install Docker on the host or rerun setup.
- `missing btrfs on host`: rerun setup to install `btrfs-progs` and ensure sudo access.
- `no host provided and no default host configured`: run `setup` in the shell or `viberun setup myhost`.
- `container image architecture mismatch`: delete and recreate the app (`rm <app>`).
- `proxy is not configured`: run `proxy setup` (then retry `app <app>` and `url`).
