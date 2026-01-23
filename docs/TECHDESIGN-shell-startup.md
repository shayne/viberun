# Technical Design: Shell Startup Port Forwards + Vibe/Open

## Overview
The shell startup flow will become the single place where we connect to the host, fetch app state, establish port forwards, and render a friendly summary before showing the prompt. This eliminates deferred sync logic and makes URLs immediately available.

## Current State
- Port forwards are created only when opening an interactive PTY session (run/shell).
- App list and gateway readiness are asynchronous and gated by `RequiresSync`.
- Users must "run" an app before localhost URLs work.

## Proposed Flow
1. Shell start -> show in-place status line (TTY only).
2. Resolve host config and connect gateway.
3. Fetch app list and status.
4. For each app, resolve host port and establish local forward.
5. Render app list and banner, then show prompt.

## Components to Change

### Shell startup
- Move host sync and app list loading into the initial shell startup.
- Replace `RequiresSync` gating with a single startup sync that sets:
  - `state.gateway`
  - `state.apps`
  - `state.appsLoaded`
  - `state.appsSyncing = false`
- Provide a single failure path that renders a friendly error and still shows the prompt.

### Status line UI
- Add a startup renderer that updates a single line in place.
- Use braille-style spinner frames (Unicode braille pattern) when TTY is available; fallback to plain text when not.
- Example stages:
  - Connecting to host
  - Syncing apps
  - Forwarding app ports

### App status + URL model
- Add a new gateway command to determine running state:
  - `viberun-server <app> status` -> `running|stopped|missing` (or JSON if we want to extend later).
- For URL display:
  - If forward established: `http://localhost:<port>`
  - If proxy configured, optionally include the public URL as a secondary field.

### Port forward manager
- Add a shell-scoped manager that:
  - Starts forwards after sync.
  - Tracks which ports are bound.
  - Closes all listeners on shell exit.
- Reuse `startLocalForwardMux` and `ensureLocalPortAvailable`.
- Skip forward if:
  - host is local
  - container missing
  - port already in use

### Command changes
- Rename `run` to `vibe` in the command registry and help output.
- Keep `run` as a hidden alias for compatibility.
- Add `open` command:
  - Global: `open <app>`
  - App context: `open`
  - Implementation: resolve URL via existing proxy info or local forward port, then call `openURL`.

## Error Handling
- Gateway connect failure: show error banner, skip app list/forwards, prompt still appears.
- Status query failure: mark app status as `unknown`.
- Port forward failure: mark URL as `unavailable` and add a short summary line.

## Data Flow
1. Shell start -> resolve host.
2. `startGateway` -> `gateway` stored in state.
3. `runRemoteAppsList` -> app names.
4. For each app:
   - `viberun-server <app> status`
   - `viberun-server <app> port`
   - `startLocalForwardMux` if host is remote
5. Render table.

## Testing Strategy
- Unit tests for:
  - status parsing and display formatting
  - forward manager lifecycle (start/stop, port in use)
- Integration test:
  - start shell, verify localhost port is open before running the app

## Rollout
- Gate the new startup flow behind a feature flag for one release if needed.
- Keep `run` as alias for at least one release.
