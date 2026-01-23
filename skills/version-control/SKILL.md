---
name: version-control
description: Help users configure Git in viberun containers, including git identity, SSH agent forwarding, GitHub CLI (gh) HTTPS auth, and basic tmux guidance. Use when users ask about git, GitHub auth, ssh keys, or version control setup inside containers.
metadata:
  short-description: Git + GH setup
---

# version-control

Purpose: help users quickly use git in app containers without auto-auth.

## Version freshness
- Knowledge cutoff is 2024 and the current date is 2026. Do not assume “latest” versions from memory.
- When users ask about latest Git/GH versions or APIs, verify with `git --version`, `gh --version`, `mise ls-remote`, or `brew info` instead of guessing.
- For API/flag details, prefer `--help` output or current docs instead of memory.

## What exists by default
- Git, SSH, and the GitHub CLI (`gh`) are installed in containers.
- A host-managed config file is mounted at `/opt/viberun/userconfig.json` and applied on startup.
  - This seeds `git config --global user.name` and `user.email` if present in the host config.

## Workflow
1) Check identity: `git config --global user.name` and `git config --global user.email`.
2) Ask which auth path they want: **SSH agent forwarding** or **HTTPS via gh**.
3) If SSH:
   - From laptop: `VIBERUN_FORWARD_AGENT=1 viberun`, then `vibe <app>`.
   - For existing apps, run `app <app>` then `update` to recreate the container with the agent socket mounted.
   - Inside container: `echo "$SSH_AUTH_SOCK"` and `ssh -T git@github.com` to verify.
4) If HTTPS:
   - `gh auth login` (choose HTTPS when prompted).
   - `gh auth setup-git` to configure Git credential usage.
   - Verify with `gh auth status`.
5) If the user needs a fresh shell to complete login flows, explain tmux:
   - New window: Ctrl-b then `c`
   - Switch windows: Ctrl-b then `n`/`p`
   - Close a window: `exit`

## Notes
- If the git identity is missing, update it locally and restart the container (or run `app <app>` then `update`) so the startup config applies.
- Do not auto-auth; always ask which path the user prefers.
