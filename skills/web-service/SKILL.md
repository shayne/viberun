---
name: web-service
description: Help run a web service under vrctl+s6, including ports and health checks.
metadata:
  short-description: Web service
---

# web-service

Purpose: help set up a web app inside the container with a stable vrctl-managed service and port 8080 mapping.

## Workflow
1) Identify the app command and working directory.
2) If the user asks for a simple web app without specifics, pick a lightweight server and build a small, tasteful single-page HTML UI.
3) Ensure the web server binds to `0.0.0.0` (not `127.0.0.1`) so host port mapping works.
4) Register the service with `vrctl service add <app> --cmd "<start command>" --cwd /workspace/<app> --env PORT=8080 --env HOST=0.0.0.0`.
5) Verify with `vrctl service status <app>` and `vrctl service logs <app> -n 200`.
6) Mention that vrctl keeps the service running on container restarts.

## vrctl template
```
vrctl service add <app> \
  --cmd "<start command>" \
  --cwd /workspace/<app> \
  --env PORT=8080 \
  --env HOST=0.0.0.0
```

## Common checks
- `vrctl service status <app>`
- `vrctl service logs <app> -n 200`

## User-facing notes
- Treat the host port as the only user-facing port; do not mention 8080 unless the user explicitly asks.
- Always include the concrete local URL derived from the environment.
- Use `printenv VIBERUN_HOST_PORT` (or `echo "$VIBERUN_HOST_PORT"`) to read it, then say:
  - `Open http://localhost:<port> in your laptop browser while this session is active.`
- After verifying the service is responding, read `VIBERUN_HOST_PORT` (e.g., `port="$(printenv VIBERUN_HOST_PORT)"`) and run `xdg-open "http://localhost:${port}"` once.

## Default hello-world behavior
- Prefer a minimal single-file app with an attractive HTML + CSS landing page.
- Keep dependencies light (Python stdlib or Node + minimal script).
- Use port 8080 and bind `0.0.0.0` automatically; do not ask the user to specify these or mention them unless asked.
