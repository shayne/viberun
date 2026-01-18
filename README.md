<p align="center">
  <img src="assets/viberun-logo.png" alt="viberun logo" width="240">
</p>

# viberun

`viberun` is a CLI-first, agent-native app host. Run `viberun <app>` locally and get dropped into an agent session inside a persistent Ubuntu container on a remote host. Containers keep their filesystem state between sessions and can run long-lived services.

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

### 2) Bootstrap a host (once per VM)

Use any SSH host alias (for example, `myhost` in `~/.ssh/config`):

```bash
viberun bootstrap myhost
```

Optional: set it as your default host (and agent) so you can omit `@host` later:

```bash
viberun config --host myhost --agent codex
```

### 3) Start your first app

```bash
viberun hello-world
```

If this is the first run, the server will prompt to create the container. Press Enter to accept.

Detach without stopping the agent: Ctrl-\\ . Reattach later with `viberun hello-world`.

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

## Common commands

```bash
viberun myapp
viberun myapp@hostb
viberun myapp shell
viberun myapp snapshot
viberun myapp snapshots
viberun myapp restore latest
viberun myapp --delete -y
viberun bootstrap [<host>]
viberun config --host myhost --agent codex
```

<details>
<summary>Table of contents</summary>

- [Quick start (end-to-end)](#quick-start-end-to-end)
- [Common commands](#common-commands)
- [Development](#development)
- [Architecture](#architecture)
  - [High-level flow](#high-level-flow)
  - [Core components](#core-components)
  - [Session lifecycle](#session-lifecycle)
  - [Bootstrap pipeline](#bootstrap-pipeline)
  - [Networking and ports](#networking-and-ports)
  - [Snapshots and restore](#snapshots-and-restore)
  - [Host RPC bridge](#host-rpc-bridge)
  - [Configuration and state](#configuration-and-state)
  - [Agents](#agents)
  - [Security model](#security-model)
  - [Repository layout](#repository-layout)
  - [Troubleshooting](#troubleshooting)

</details>

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
```

### E2E and integration

```bash
bin/viberun-e2e-local
bin/viberun-integration
```

### Bootstrap in development

When you run `viberun` via `go run` (or a dev build), bootstrap defaults to staging the local server binary and building the container image locally. This is equivalent to:

```bash
viberun bootstrap --local --local-image myhost
```

Under the hood, it builds a `viberun:dev` image for the host architecture, streams it over `ssh` with `docker save | docker load`, and tags it as `viberun:latest` on the host.

If you want to explicitly pass a local server binary:

```bash
mise exec -- go build -o /tmp/viberun-server ./cmd/viberun-server
viberun bootstrap --local-path /tmp/viberun-server myhost
```

For the full build/test/E2E flow, see `DEVELOPMENT.md`.

## Architecture

### High-level flow

```
viberun <app>
  -> ssh <host>
    -> viberun-server <app>
      -> docker container viberun-<app>
        -> agent session (tmux)

container port 8080
  -> host port (assigned per app)
    -> ssh -L localhost:<port>
      -> http://localhost:<port>
```

### Core components

- Client: `viberun` CLI on your machine.
- Server: `viberun-server` executed on the host via SSH (no long-running daemon required).
- Container: `viberun-<app>` Docker container built from the `viberun:latest` image.
- Agent: runs inside the container in a tmux session (default provider: `codex`).
- Host RPC: local Unix socket used by the container to request snapshot/restore operations.

### Session lifecycle

1. `viberun <app>` resolves the host (from `@host` or your default config) and runs `viberun-server` over SSH.
2. The server creates the container if needed, or starts it if it already exists.
3. The agent process is attached via `docker exec` inside a tmux session so it persists across disconnects.
4. `viberun` sets up a local port forward so you can open the app on `http://localhost:<port>`.

### Bootstrap pipeline

The bootstrap script (run on the host) does the following:

- Verifies the host is Ubuntu.
- Installs Docker (if missing) and enables it.
- Pulls the `viberun` container image from GHCR (unless using local image mode).
- Downloads and installs the `viberun-server` binary.

Useful bootstrap overrides:

- `VIBERUN_SERVER_REPO`: GitHub repo for releases (default `shayne/viberun`).
- `VIBERUN_SERVER_VERSION`: release tag or `latest`.
- `VIBERUN_IMAGE`: container image override.
- `VIBERUN_SERVER_INSTALL_DIR`: install directory on the host.
- `VIBERUN_SERVER_BIN`: server binary name on the host.
- `VIBERUN_SERVER_LOCAL_PATH`: use a local server binary staged over SSH.
- `VIBERUN_SKIP_IMAGE_PULL`: skip pulling from GHCR (used for local builds).

### Networking and ports

- Each app container exposes port `8080` internally.
- The host port is assigned per app (starting at `8080`) and stored in the host server state.
- `viberun` opens an SSH local forward so `http://localhost:<port>` connects to the host port.

### Snapshots and restore

Snapshots are Docker commits stored as images named `viberun-snapshot-<app>:<tag>`.

- `viberun <app> snapshot` creates a timestamped snapshot.
- `viberun <app> snapshots` lists snapshots.
- `viberun <app> restore <snapshot|latest>` restores from a snapshot.
- `viberun <app> --delete -y` removes the container and snapshots.

### Host RPC bridge

When you open a session, the server creates a Unix socket on the host and mounts it into the container at `/var/run/viberun-hostrpc`. The container uses it to request snapshot and restore operations. Access is protected by a per-session token file mounted alongside the socket.

### Configuration and state

Local config lives at `~/.config/viberun/config.json` (or `$XDG_CONFIG_HOME/viberun/config.json`) and stores:

- `default_host`
- `agent_provider`
- `hosts` (alias mapping)

Host server state lives at `~/.config/viberun/server-state.json` (or `$XDG_CONFIG_HOME/viberun/server-state.json`) and stores the port mapping for each app.

### Agents

Supported agent providers:

- `codex` (default)
- `claude`
- `gemini`

Set globally with `viberun config --agent <provider>` or per-run with `viberun --agent <provider> <app>`.

### Security model

- All control traffic goes over SSH; the server is invoked on demand and does not expose a network port.
- The host RPC socket is local-only and protected by filesystem permissions and a per-session token.
- Containers are isolated by Docker and only the app port is exposed.

### Repository layout

- `cmd/`: Go entrypoints (`viberun`, `viberun-server`).
- `internal/`: Core packages (config, server state, SSH args, target parsing, TUI helpers).
- `bin/`: Helper scripts for installs, integration/E2E flows, and container utilities.
- `skills/`: Codex skill definitions used inside containers.
- `config/`: Shell/TMUX/Starship config and runtime assets.
- `Dockerfile`: Base container image definition.

### Troubleshooting

- `unsupported OS: ... expected ubuntu`: bootstrap currently supports Ubuntu only.
- `docker is required but was not found in PATH`: install Docker on the host or re-run bootstrap.
- `no host provided and no default host configured`: run `viberun config --host myhost` or use `myapp@host`.
- `container image architecture mismatch`: delete and recreate the app (`viberun <app> --delete -y`).
