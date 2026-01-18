# viberun App Environment

This AGENTS.md applies to the app working directory inside the container.

## Environment
- Persistent Ubuntu container for a single app.
- Workdir: /home/viberun/app (put code here).
- Home: /home/viberun (dotfiles live under /home/viberun/.codex).
- Default user: viberun (use sudo only for apt/apt-get installs).

## Services and ports
- Use vrctl to add/start/stop/restart services. Do not use systemd or s6 directly.
- Web services must bind to 0.0.0.0:8080 and run in the foreground.
- Tell the user to open: http://localhost:$VIBERUN_HOST_PORT (while the session is active).
- Default behavior: when the user asks for a web app, start it as a vrctl-managed service unless they explicitly say not to.
- vrctl uses exec-style commands: pass the executable via `--cmd` and each argument via `--arg` (no shell strings).

## Packages and tooling
- Install OS packages with sudo apt or sudo apt-get.
- Node 22+ and Python 3 are available.
- Prefer Python + uv or TypeScript per the skills.

## Skills (invoke with $name)
- $service-management: install deps and manage long-running services.
- $web-service: run a web app on port 8080 with vrctl.
- $background-service: run background processes under vrctl.
- $cron-jobs: set up cron via vrctl.

## Snapshots and safety
- Before risky or destructive changes, ask if the user wants a snapshot.
- If host RPC is available, run `vrctl host snapshot` and report the ref.
- Otherwise, ask the user to run `viberun <app> snapshot` locally.
- Consider snapshots before large refactors, dependency upgrades, or data migrations.
- To review snapshots inside the container, run `vrctl host snapshots` (shows version tags and timestamps).
- To roll back, use `vrctl host restore latest` or `vrctl host restore vN`.
- Restore will briefly detach from tmux and then reconnect once the container is back up.

## User experience
- Keep instructions simple and action-oriented.
- After starting services, verify status/logs and share the local URL.
