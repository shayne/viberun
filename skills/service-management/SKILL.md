---
name: service-management
description: Manage long-running services inside a viberun container. Use when the user asks to install dependencies (via apt/apt-get), run or supervise background services (web servers, Redis, Postgres), inspect logs/status, or set up service startup commands with vrctl.
---

# Service Management

## Overview

Use `vrctl` to supervise services (start/stop/restart/status/logs). Install packages with `sudo apt`/`sudo apt-get` and run services under the non-root `viberun` user. Avoid systemd inside the container.

## Quick Start

Install packages:

```sh
sudo apt-get update
sudo apt-get install -y redis-server postgresql
```

Create services:

```sh
vrctl service add web --cmd "node server.js" --cwd /home/viberun/app
vrctl service add redis --cmd "redis-server --bind 0.0.0.0"
```

Check status and logs:

```sh
vrctl service status web
vrctl service logs web -n 200
```

## Common Tasks

Create a service with env:

```sh
vrctl service add api --cmd "npm run start" --cwd /home/viberun/app --env NODE_ENV=production --env PORT=8080
```

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
