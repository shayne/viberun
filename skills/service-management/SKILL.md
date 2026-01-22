---
name: service-management
description: Manage long-running services inside a viberun container. Use when the user asks to install dependencies (via mise or brew), run or supervise background services (web servers, Redis, Postgres), inspect logs/status, or set up service startup commands with vrctl.
---

# Service Management

## Overview

Use `vrctl` to supervise services (start/stop/restart/status/logs). Install tools with `mise` first and Homebrew second. The mise config file lives at `/home/viberun/app/.mise.toml`. Run services under the non-root `viberun` user. Avoid systemd inside the container.
When possible, use dev servers with reload/watch so changes apply without restarting services.

## Version freshness
- Knowledge cutoff is 2024 and the current date is 2026. Do not assume “latest” versions from memory.
- When installing/pinning tools, verify via `mise ls-remote <tool>` and/or `brew info <formula>` before choosing versions.
- For API/flag details, prefer `--help` output or current docs instead of memory.

## Quick Start

Install packages:

```sh
mise install
brew install redis postgresql
```

Create services:

```sh
vrctl service add web --cmd node --arg server.js --cwd /home/viberun/app
vrctl service add redis --cmd mise --arg exec --arg -C --arg /home/viberun/app --arg -- --arg redis-server --arg --bind --arg 0.0.0.0
```

Check status and logs:

```sh
vrctl service status web
vrctl service logs web -n 200
```

## Common Tasks

Create a service with env:

```sh
vrctl service add api --cmd npm --arg run --arg start --cwd /home/viberun/app --env NODE_ENV=production --env PORT=8080
```

Run a service with a mise-managed tool:

```sh
vrctl service add worker \
  --cmd mise \
  --arg exec \
  --arg -C --arg /home/viberun/app \
  --arg -- \
  --arg php --arg /home/viberun/app/worker.php
```

Notes:
- `mise exec -C /home/viberun/app -- <cmd>` ensures mise loads `/home/viberun/app/.mise.toml` and puts the right tool on PATH for the service.
- If you define a task in `/home/viberun/app/.mise.toml`, you can run it with `mise run <task>` and set `--cwd /home/viberun/app` on the service.
- Use `mise use <tool>@<version>` or edit `/home/viberun/app/.mise.toml`, then run `mise install`.

Restart or stop a service:

```sh
vrctl service restart api
vrctl service stop redis
```

Remove a service:

```sh
vrctl service remove redis
```

## Notes

- Service definitions live under `~/.local/services`. Logs go to `~/.local/logs/<name>.log`.
- Packages may install systemd units; ignore them and manage processes with `vrctl`.
- Prefer data directories under `/home/viberun/app` so services can write without root.
- If you want to avoid wrapping commands with `mise exec`, add the shims directory to PATH: `~/.local/share/mise/shims`. For vrctl, pass it via `--env PATH=...`.
- For brew-installed tools, services may not inherit PATH. Use `--env PATH=/home/viberun/.local/bin:/home/viberun/.linuxbrew/bin:/home/viberun/.linuxbrew/sbin:/usr/local/bin:/usr/bin:/bin` or call the full path to the binary.
- If restart hits “address already in use”, use `vrctl service stop <name>`, wait 1–2s, then `vrctl service start <name>`.
- Use `lsof -nP -iTCP:<port> -sTCP:LISTEN` to confirm the port holder before killing processes; ask the user first.
