---
name: background-service
description: Help run a long-lived background process under vrctl.
metadata:
  short-description: Background service
---

# background-service

Purpose: help run a long-lived background process under vrctl.

## Workflow
1) Define the command and working directory.
2) Register the service with `vrctl service add`.
3) Start it (vrctl starts by default).
4) Check logs and restart policy.
5) Mention that vrctl keeps the service running on container restarts.
6) Use `vrctl service restart` when the command/cwd/env are unchanged.

## vrctl template
```
vrctl service add <name> \
  --cmd "<command>" \
  --cwd /home/viberun/app
```

## Common checks
- `vrctl service status <name>`
- `vrctl service logs <name> -n 200`

## Language/tooling preferences
- Prefer typed Python (type annotations) and use uv exclusively for env/deps and running (`uv init`, `uv add`, `uv run`); avoid pip/venv directly.
- Or use Node with TypeScript only (.ts). The base image ships a recent Node (22+) so prefer native TypeScript execution with `node`. If that fails, run TS via `npx -y tsx` (still TypeScript).
- Never use plain JavaScript unless the user explicitly asks for it.

## Guardrails
- Do not call s6 or manage supervisors directly.
- If vrctl reports the supervisor is not listening, wait briefly and retry once. If it still fails, ask the user to restart the app session.

## User-facing notes
- Keep explanations non-technical; do not mention vrctl or s6 unless the user asks.

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
