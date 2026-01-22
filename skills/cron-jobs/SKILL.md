---
name: cron-jobs
description: Help set up and manage cron-based jobs for recurring tasks.
metadata:
  short-description: Cron jobs
---

# cron-jobs

Purpose: help schedule recurring jobs inside the container using a user-space cron runner (no sudo).

## Version freshness
- Knowledge cutoff is 2024 and the current date is 2026. Do not assume “latest” versions from memory.
- When installing/pinning tools, verify via `mise ls-remote <tool>` and/or `brew info <formula>` before choosing versions.
- For API/flag details, prefer `--help` output or current docs instead of memory.

## Workflow
1) Install a cron runner with Homebrew (preferred) or mise if already pinned in `/home/viberun/app/.mise.toml`.
2) Put the schedule in a file under `/home/viberun/app` or `~/.config/cron`.
3) Run the cron runner in the foreground under vrctl.
4) Verify execution via logs under `~/.local/logs`.
5) Mention that vrctl keeps cron running on container restarts.

## Install + start cron (supercronic preferred)
```
brew install supercronic
cat > /home/viberun/app/cron.tab <<'EOF'
SHELL=/bin/bash
PATH=/home/viberun/.local/bin:/home/viberun/.linuxbrew/bin:/usr/local/bin:/usr/bin:/bin

# m h dom mon dow command
*/5 * * * * cd /home/viberun/app && ./scripts/job.sh >> /home/viberun/.local/logs/cron.log 2>&1
EOF

vrctl service add cron --cmd supercronic --arg /home/viberun/app/cron.tab
```

If a cron command depends on mise-managed tools, wrap it:
```
*/5 * * * * mise exec -C /home/viberun/app -- <cmd> >> /home/viberun/.local/logs/cron.log 2>&1
```

## If supercronic is unavailable
- `brew install cronie`
- Use `crond --help` to choose the foreground flag (`-n` or `-f`) and log flags.
- Point `crond` at a cron directory under `~/.config/cron` and keep logs under `~/.local/logs/`.

## Verify
- `vrctl service status cron`
- `tail -n 200 /home/viberun/.local/logs/cron.log`

## Language/tooling preferences
- Prefer typed Python (type annotations) and use uv exclusively for env/deps and running (`uv init`, `uv add`, `uv run`); avoid pip/venv directly.
- Or use Node with TypeScript only (.ts). The base image ships a recent Node (22+) so prefer native TypeScript execution with `node`. If that fails, run TS via `npx -y tsx` (still TypeScript).
- Never use plain JavaScript unless the user explicitly asks for it.

## User-facing notes
- Keep explanations non-technical; only mention vrctl if the user explicitly asks how services are managed.

## Snapshot guidance
- If the user asks to save progress, run `vrctl host snapshot` and report the snapshot ref.
- Before risky or destructive changes (removing services, deleting data, large refactors), ask the user if they want a snapshot.
  - If they say yes and host snapshots are available, run `vrctl host snapshot`.
  - If host snapshots are unavailable, ask the user to run `viberun <app> snapshot` from their machine.
- If they say no, proceed without snapshot and note it briefly.
- If the user asks to roll back, run `vrctl host snapshots` to list available tags, then `vrctl host restore <ref>`.
  - Accept either a tag or full ref; if given a tag, pass it as-is and let the host resolve it.
  - Restore will disconnect the session; tell the user to re-run `viberun <app>` and then re-check service status / bring it back up.
  - If restore is unavailable, ask the user to run `viberun <app> restore <ref>` from their machine.
