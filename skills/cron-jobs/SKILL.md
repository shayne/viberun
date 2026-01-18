---
name: cron-jobs
description: Help set up and manage cron-based jobs for recurring tasks.
metadata:
  short-description: Cron jobs
---

# cron-jobs

Purpose: help schedule recurring jobs inside the container using cron.

## Workflow
1) Ensure cron is installed and running under vrctl.
2) Choose either `/etc/cron.d/` (system-wide) or user crontab.
3) Add the schedule with explicit PATH and shell.
4) Verify execution via logs.
5) Mention that vrctl keeps cron running on container restarts.

## Install + start cron
```
apt-get update
apt-get install -y cron
vrctl service add cron --cmd "cron -f"
```

## /etc/cron.d/ example
```
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# m h dom mon dow user command
*/5 * * * * root /usr/bin/env bash -lc 'cd /app && ./scripts/job.sh' >> /var/log/<app>-cron.log 2>&1
```

## Verify
- `vrctl service status cron`
- `tail -n 200 /var/log/<app>-cron.log`

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
