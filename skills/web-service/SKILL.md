---
name: web-service
description: Help run a web service under vrctl, including ports and health checks.
metadata:
  short-description: Web service
---

# web-service

Purpose: help set up a web app inside the container with a stable vrctl-managed service and port 8080 mapping.

## Workflow
1) Identify the app command and working directory.
2) If the user asks for a simple web app without specifics, pick a lightweight server and build a small, tasteful single-page HTML UI.
3) Ensure the web server binds to `0.0.0.0` (not `127.0.0.1`) so host port mapping works.
4) Register the service with `vrctl service add <app> --cmd "<start command>" --cwd /home/viberun/app --env PORT=8080 --env HOST=0.0.0.0`.
5) Wait briefly (1–2s), then verify with `vrctl service status <app>` and `vrctl service logs <app> -n 200`.
6) Mention that vrctl keeps the service running on container restarts.

## vrctl template
```
vrctl service add <app> \
  --cmd "<start command>" \
  --cwd /home/viberun/app \
  --env PORT=8080 \
  --env HOST=0.0.0.0
```

## Common checks
- `vrctl service status <app>`
- `vrctl service logs <app> -n 200`

## Service setup checklist (do this in order)
1) Ensure `/home/viberun/app` exists and contains the server entrypoint.
2) Use a foreground command (no daemonize/no nohup).
3) Add the service once with `vrctl service add` (include `--cwd` + `--env PORT=8080 --env HOST=0.0.0.0`).
4) If the service exists and you changed the command/cwd/env, remove then re-add. Otherwise use `vrctl service restart`.
5) After add/restart, check status + logs.

## Common failure and recovery
- If the service exits due to app errors, fix the app then run `vrctl service restart <app>`.
- If service start fails, wait 1–2s and retry once. Do not manage supervisors directly.
- If it still fails, report that the service manager in the container isn’t ready and ask the user to restart the app session.

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

## Language/tooling preferences
- Prefer typed Python (type annotations) and use uv exclusively for env/deps and running (`uv init`, `uv add`, `uv run`); avoid pip/venv directly.
- Or use Node with TypeScript only (.ts). The base image ships a recent Node (22+) so prefer native TypeScript execution with `node`. If that fails, run TS via `npx -y tsx` (still TypeScript).
- Never use plain JavaScript unless the user explicitly asks for it.

## Guardrails
- Do not call s6 or manage supervisors directly.
- If vrctl reports the supervisor is not listening, stop and tell the user to restart the app session instead of trying to fix s6 manually.

## User-facing notes
- Treat the host port as the only user-facing port; do not mention 8080 unless the user explicitly asks.
- Always include the concrete local URL derived from the environment.
- Use `printenv VIBERUN_HOST_PORT` (or `echo "$VIBERUN_HOST_PORT"`) to read it, then say:
  - `Open http://localhost:<port> in your laptop browser while this session is active.`
- After verifying the service is responding, read `VIBERUN_HOST_PORT` (e.g., `port="$(printenv VIBERUN_HOST_PORT)"`) and run `xdg-open "http://localhost:${port}"` once.
- Keep user-facing instructions non-technical; do not mention vrctl or s6 unless the user asks.

## Default hello-world behavior
- Prefer a minimal single-file app with an attractive HTML + CSS landing page.
- Keep dependencies light (Python stdlib or Node + minimal script).
- Use port 8080 and bind `0.0.0.0` automatically; do not ask the user to specify these or mention them unless asked.
