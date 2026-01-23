# Epics and Stories: Shell Startup + Vibe/Open

## Epic 1: Startup Sync + Status Line
Goal: shell connects to host, syncs apps, and shows progress before prompt.

Stories:
1. Status line during startup
   - As a user, I see a single-line status indicator while the shell connects.
   - Acceptance:
     - Updates in place; no extra lines are printed.
     - Includes stages: connecting, syncing apps, forwarding ports.
     - Falls back to plain text when not a TTY.

2. Gateway connection before prompt
   - As a user, I get a shell prompt only after the host connection attempt completes.
   - Acceptance:
     - Success: app list rendered then prompt.
     - Failure: friendly error message then prompt.

## Epic 2: App List + Friendly Banner
Goal: show apps with status + URL and a helpful tip.

Stories:
1. App list for existing users
   - As a returning user, I see my apps listed with status and URL.
   - Acceptance:
     - Each row includes app name, running state, URL (or unavailable).
     - If a port is in use, the row indicates "unavailable".

2. Welcome message for new users
   - As a new user, I see a short welcome and a single next step.
   - Acceptance:
     - No app list is shown when there are zero apps.
     - Suggests `vibe <name>`.

3. Banner tip
   - As a user, I see a short tip after the list.
   - Acceptance:
     - Tip references `vibe` and `open`.
     - Tip is omitted when startup fails.

## Epic 3: Port Forward Manager
Goal: forward app ports on shell start.

Stories:
1. Forward ports for running apps
   - As a user, my app URLs work immediately after starting the shell.
   - Acceptance:
     - For remote hosts, forwards are created for running apps.
     - For local hosts, no forwards are created.

2. Cleanup on exit
   - As a user, forwards are released when I exit the shell.
   - Acceptance:
     - All listeners close on exit.

## Epic 4: Vibe/Open Commands
Goal: rename run to vibe and add open.

Stories:
1. Rename run to vibe
   - As a user, I type `vibe <app>` to start a session.
   - Acceptance:
     - Help shows `vibe` instead of `run`.
     - `run` still works as a hidden alias.

2. Open command
   - As a user, I can type `open <app>` to open the browser.
   - Acceptance:
     - Works in global scope and app scope.
     - Uses public URL when available; otherwise uses localhost.

## Epic 5: App Status
Goal: show running status reliably.

Stories:
1. Server status command
   - As a user, I see accurate running status in the app list.
   - Acceptance:
     - `viberun-server <app> status` returns running/stopped/missing.
     - Shell uses this result to render status.
