# viberun App Environment

This AGENTS.md applies to the app working directory inside the container.

## Environment
- Persistent Ubuntu container for a single app.
- Workdir: /home/viberun/app (put code here).
- Home: /home/viberun (persisted boundary for code, data, and services).
- Default user: viberun (no sudo; system package installs are disabled).

## Services and ports
- Use vrctl to add/start/stop/restart services. Do not use systemd or s6 directly.
- Web services must bind to 0.0.0.0:8080 and run in the foreground.
- Tell the user to open: http://localhost:$VIBERUN_HOST_PORT (while the session is active).
- Default behavior: when the user asks for a web app, start it as a vrctl-managed service unless they explicitly say not to.
- vrctl uses exec-style commands: pass the executable via `--cmd` and each argument via `--arg` (no shell strings).

## Packages and tooling
- Use mise first and Homebrew second for tools and packages. The mise config lives at `/home/viberun/app/.mise.toml`.
- Homebrew installs under `/home/viberun/.linuxbrew` (inside the snapshot boundary).
- Node 24 LTS and Python 3 are available (`python` points to Python 3).
- Prefer Python + uv or TypeScript per the skills.

## Version freshness
- Knowledge cutoff is 2024 and the current date is 2026. Do not assume “latest” versions from memory.
- When pinning or upgrading tools, verify versions with `mise ls-remote <tool>` and `brew info <formula>` (or `brew search`), and use those results before editing `.mise.toml`.
- For API/flag details, prefer `--help` output or current docs instead of memory.

## Skills (invoke with $name)
- $service-management: install deps and manage long-running services.
- $web-service: run a web app on port 8080 with vrctl.
- $background-service: run background processes under vrctl.
- $cron-jobs: set up cron via vrctl.

## Skills layout
- Base skills are shipped in /opt/viberun/skills and symlinked into each agent's skills directory.
- User skills should be added directly under the agent's skills directory (for example, /home/viberun/.codex/skills).
- Base skill symlinks are refreshed on container start; user skills are left untouched.

## Snapshots and safety
- Before risky or destructive changes, ask if the user wants a snapshot.
- If host RPC is available, run `vrctl host snapshot` and report the ref.
- Otherwise, ask the user to run `app <app>` then `snapshot` in the viberun shell.
- Consider snapshots before large refactors, dependency upgrades, or data migrations.
- To review snapshots inside the container, run `vrctl host snapshots` (shows version tags and timestamps).
- To roll back, use `vrctl host restore latest` or `vrctl host restore vN`.
- Restore will briefly detach from tmux and then reconnect once the container is back up.

## Branch environments
- Branch environments are managed by viberun, not git. `/home/viberun/app` might not be a git repo.
- Current context is available via `$VIBERUN_APP`. If it contains `--`, you are in a branch container.
- Natural language mapping:
  - “Apply this branch” → if `$VIBERUN_APP` has `--`, run `apply` immediately (no git checks, no extra prompts). On success it auto-switches to the base app. Otherwise list branches and apply if there is exactly one; if multiple, ask which.
  - “Create a branch <name>” → `vrctl host branch create <name> --attach` (auto-switch).
  - “Delete branch <name>” → confirm, then `vrctl host branch delete <name> --attach`.
  - “Delete this branch” (while `$VIBERUN_APP` contains `--`) → confirm, then parse the branch name from `$VIBERUN_APP` and run `vrctl host branch delete <branch> --attach` (auto-switches to base).
- List branches (from base app container): `vrctl host branch list`.
- Apply back to base:
  - From the branch container: `apply` (no args, auto-switches to base).
  - From the base container: `apply <branch>` or `vrctl host branch apply <branch> --attach`.
- When the user explicitly asks to apply, do not prompt for snapshots; execute the apply and report errors if they occur.
- Conflicts: use `$branch-apply-conflicts` and resolve markers in `/home/viberun/app`, then rerun apply.
- Data changes are not promoted; migrations must live in `app/`.

## User experience
- Keep instructions simple and action-oriented.
- After starting services, verify status/logs and share the local URL.
