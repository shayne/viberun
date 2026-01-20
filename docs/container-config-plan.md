# Container/User Config Consolidation Plan

## Goals
- Replace scattered per-app env vars with a single, host-managed config file mounted into containers.
- Keep session-only values as env vars (agent forwarding, xdg-open socket, terminal colors, etc.).
- Preserve current behavior by exporting legacy env vars from the entrypoint so existing scripts/skills continue to work.
- Allow breaking changes in internal wiring while keeping user-visible behavior the same.

## Current env var inventory (viberun-specific)
Below are the viberun env vars found in the repo and where they are used. This is the basis for deciding what moves into config files.

### Container/runtime-facing (candidates for containerconfig)
- `VIBERUN_APP` (tmux status, vrctl, agent UI)
- `VIBERUN_CONTAINER` (container identification)
- `VIBERUN_APP_PORT` (app listens on 8080 inside container)
- `VIBERUN_HOST_PORT` (tmux status + skills + templates)
- `VIBERUN_PORT` (redundant with host port)
- `VIBERUN_PUBLIC_URL` / `VIBERUN_PUBLIC_DOMAIN` (tmux status + skills)
- `VIBERUN_HOST_RPC_SOCKET` / `VIBERUN_HOST_RPC_TOKEN_FILE` (vrctl)
- `VIBERUN_WEB_PORT` (tmux status, default 8080)

### Session-only / transient (keep as env)
- `VIBERUN_AGENT` (session label for agent selection)
- `VIBERUN_AGENT_CHECK` (agent shims/health checks)
- `VIBERUN_FORWARD_AGENT` (local flag, not container state)
- `VIBERUN_XDG_OPEN_SOCKET` (local-forward socket per session)
- `SSH_AUTH_SOCK` (forwarded socket path)
- `TERM`, `COLORTERM`, `NO_COLOR` (terminal/session)

### Local install / host config (not containerconfig)
- `VIBERUN_IMAGE`, `VIBERUN_PROXY_IMAGE`, `VIBERUN_SKIP_IMAGE_PULL`
- `VIBERUN_SERVER_*`, `VIBERUN_INSTALL_*`, `VIBERUN_REPO`, `VIBERUN_VERSION`
- `VIBERUN_PROXY_CONFIG_PATH` (server-side config path)
- `VIBERUN_AUTO_CREATE`, `VIBERUN_AUTH_BUNDLE` (one-time transport)

### User identity (userconfig)
- `git user.name`, `git user.email` (currently seeded via host-managed userconfig)

## Proposed config files
### 1) User config (global)
- Host path: `/var/lib/viberun/userconfig.json`
- Container mount: `/opt/viberun/userconfig.json` (read-only)
- Ownership: root on host; read-only mount into container

Example:
```json
{
  "git": {
    "name": "Ada Lovelace",
    "email": "ada@example.com"
  }
}
```

### 2) Container config (per app)
- Host path: `/var/lib/viberun/apps/<app>/containerconfig.json`
- Container mount: `/opt/viberun/containerconfig.json` (read-only)

Example:
```json
{
  "app": {
    "name": "myapp",
    "container": "viberun-myapp"
  },
  "ports": {
    "app": 8080,
    "host": 4242,
    "web": 8080
  },
  "hostrpc": {
    "socket": "/var/run/viberun-hostrpc/rpc.sock",
    "token_file": "/var/run/viberun-hostrpc/token"
  },
  "proxy": {
    "public_url": "https://myapp.example.com",
    "public_domain": "myapp.example.com"
  }
}
```

## What moves to containerconfig (and how we keep compatibility)
`viberun-env` loads `/opt/viberun/containerconfig.json` and exports legacy env vars before running tmux/agent commands, so existing scripts keep working.

| Legacy env var | New config field | viberun-env export | Notes |
| --- | --- | --- | --- |
| `VIBERUN_APP` | `app.name` | yes | used by tmux status + vrctl |
| `VIBERUN_CONTAINER` | `app.container` | yes | used for diagnostics |
| `VIBERUN_APP_PORT` | `ports.app` | yes | stable 8080 |
| `VIBERUN_HOST_PORT` | `ports.host` | yes | used by tmux status + skills |
| `VIBERUN_PORT` | `ports.host` | yes | keep for compatibility |
| `VIBERUN_WEB_PORT` | `ports.web` | yes | keep default 8080 |
| `VIBERUN_PUBLIC_URL` | `proxy.public_url` | yes | used by tmux status + skills |
| `VIBERUN_PUBLIC_DOMAIN` | `proxy.public_domain` | yes | used by skills |
| `VIBERUN_HOST_RPC_SOCKET` | `hostrpc.socket` | yes | used by vrctl |
| `VIBERUN_HOST_RPC_TOKEN_FILE` | `hostrpc.token_file` | yes | used by vrctl |

## Migration strategy (implementation steps)
1) **Define schema types**
   - Add `internal/containerconfig` with struct definitions and helpers.
   - Extend existing `internal/userconfig` types if needed.

2) **Generate containerconfig on the host**
   - In `cmd/viberun-server`, build the per-app config using:
     - app name and container name
     - resolved host port
     - static app/web port 8080
     - hostrpc socket/token paths
     - proxy public URL/domain (if configured)
   - Write to `/var/lib/viberun/apps/<app>/containerconfig.json`.

3) **Mount into the container**
   - Add a read-only mount for `/opt/viberun/containerconfig.json` in `dockerRunArgs`.
   - Keep existing hostrpc dir mount and home volume mount.

4) **viberun-env wrapper applies config**
   - Entry-point reads `/opt/viberun/userconfig.json` and applies git identity idempotently.
   - Add `/usr/local/bin/viberun-env` to load `/opt/viberun/containerconfig.json` and export legacy env vars.
   - Docker exec launches tmux/agent via `viberun-env` so tmux status sees the vars.

5) **Reduce env injection at container create**
   - Remove `-e VIBERUN_*` entries in `dockerRunArgs` for fields now sourced from config.
   - Continue passing session-only env (agent label, xdg-open socket, SSH auth) via `docker exec`.

6) **Update scripts/tests/docs**
   - Adjust `docker_args_test.go` to assert the config file mount instead of env var injections.
   - Keep `bin/vrctl` and `bin/viberun-tmux-status` unchanged (they read env set by `viberun-env`).
   - Document new config locations and behavior in `README.md`.

## Behavior changes / compatibility
- **Same user behavior**: URLs, tmux status, vrctl, and skills continue to work because env vars are re-exported from config.
- **Config updates**: Changing proxy URL or ports updates the host config file immediately. Env updates take effect on the next container restart (or we can add a small `tmux set-environment` refresh later).
- **Breaking changes allowed**: This collapses multiple env vars into a single config + entrypoint flow; internal wiring changes but external behavior stays the same.

## Follow-ups (future improvements)
- Add a `viberun <app> restart` or `viberun <app> env-refresh` command to re-apply config without full recreate.
- Add a host RPC endpoint to query config from inside the container if needed.
- Consider persisting `containerconfig.json` in a structured subdir alongside other per-app state.
