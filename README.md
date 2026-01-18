# viberun

`viberun` is a CLI-first, agent-native app host. You run `viberun <app>` locally and get dropped into an agent session inside a persistent Ubuntu container on a remote host (default agent: Codex). Containers keep their filesystem state between sessions and can run long-lived services via `vrctl`.

## Quick start (end-to-end)

### 1) Install the client

```bash
curl -fsSL https://viberun.sh | bash
```

Verify:

```bash
viberun --version
```

Optional overrides (advanced):

```bash
VIBERUN_REPO=OWNER/REPO VIBERUN_VERSION=v0.1.0 bash
```

### 2) Bootstrap a host (once per VM)

Ensure you can SSH into the host (for example, `myhost` in `~/.ssh/config`). Then:

```bash
viberun bootstrap myhost
```

Optional: set it as your default host (and default agent) so you can omit `@host` later:

```bash
viberun config --host myhost --agent codex
```

### 3) Start an app session

```bash
viberun myapp
```

If this is the first run, the server will prompt to create the container. Press Enter to accept.

### 4) Use Codex to build a hello-world app

In the agent session, use a prompt like:

```
Create a beautiful hello-world web app with a simple, tasteful landing page. Keep it running as a service so I can open it from my laptop.
```

Use `vrctl service add` to keep the server running, and bind the server to `0.0.0.0` so the host port mapping works.

### 5) Open the app in your local browser

While the session is active, `viberun` starts a localhost proxy to the host port. The agent will tell you the exact `http://localhost:<port>` URL to open.

## How it works

- Client: `viberun` CLI on your machine.
- Server: `viberun-server` on the host VM (runs via SSH).
- Container: Ubuntu + s6 + agent tooling + built-in skills.

Flow: `viberun myapp` -> SSH -> server CLI -> Docker container -> agent session.

## Common commands

```bash
viberun myapp
viberun myapp@hostb
viberun myapp snapshot
viberun myapp snapshots
viberun myapp restore latest
viberun myapp shell
viberun bootstrap [<host>]
viberun config --host myhost --agent codex
```

## Development

See DEVELOPMENT.md for local setup, build/test workflow, and E2E/integration scripts.
