---
name: background-service
description: Help run a long-lived background process under vrctl+s6.
metadata:
  short-description: Background service
---

# background-service

Purpose: help run a long-lived background process under vrctl+s6.

## Workflow
1) Define the command and working directory.
2) Register the service with `vrctl service add`.
3) Start it (vrctl starts by default).
4) Check logs and restart policy.
5) Mention that vrctl keeps the service running on container restarts.

## vrctl template
```
vrctl service add <name> \
  --cmd "<command>" \
  --cwd /workspace/<app>
```

## Common checks
- `vrctl service status <name>`
- `vrctl service logs <name> -n 200`
