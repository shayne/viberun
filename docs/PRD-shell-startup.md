# PRD: Shell Startup Experience + Vibe/Open Commands

## Summary
When a user starts `viberun`, the shell should connect to the host, establish app port forwards immediately, and present a friendly, non-technical summary of apps (status + URL) before showing the prompt. The command `run` is renamed to `vibe`, and a new `open` command opens an app URL in the browser. The startup experience should include a single-line status indicator with a braille-style spinner that updates in place.

## Goals
- Make startup feel instant and friendly: a single-line status indicator, then a clear app list.
- Make app URLs available immediately (no need to enter an app session to forward ports).
- Improve discoverability with a short banner that suggests the next step.
- Rename `run` to `vibe`, and add `open` for opening the browser.

## Non-Goals
- No redesign of the app container lifecycle.
- No change to server-side app runtime behavior.
- No new onboarding or account system.

## Personas
- New user: non-technical, first time in the shell.
- Returning user: wants to see what is running and open it quickly.

## User Experience

### Startup (existing user with apps)
1. User runs `viberun`.
2. Shell shows a single-line status message that updates in place (braille spinner + text):
   - "Connecting to <host>"
   - "Syncing apps"
   - "Forwarding app ports"
3. After startup, the shell prints a friendly app list with status and URLs.
4. A short banner suggests next steps (example: "Tip: vibe <app> to jump in, or open <app> to view it.")
5. The prompt appears.

### Startup (new user / no apps)
1. User runs `viberun`.
2. Same status line behavior.
3. App list is replaced with a short welcome and a single suggested command:
   - "Welcome to viberun. Create your first app with: vibe <name>"
4. The prompt appears.

## Functional Requirements
- Establish the gateway connection before showing the prompt.
- Fetch app list and determine status (running / stopped) before showing the prompt.
- Set up local port forwards for apps on startup (skip local host).
- Show a single-line status indicator that updates in place during startup.
- Print app list (name, status, URL) after startup.
- Rename `run` -> `vibe` (visible in help). Keep `run` as a hidden alias for backward compatibility.
- Add `open` command:
  - `open <app>` (global scope) opens the browser to the app URL.
  - `open` (app context) opens the current app URL.

## Non-Functional Requirements
- Startup status line must not spam multiple lines; it should update in place.
- Startup behavior should be TTY-aware (fallback to simple text when not a TTY).
- Port forwarding should never crash the shell. Failures should be reported once and the shell should still load.

## Error Handling
- If the gateway cannot connect, show a short error and keep the shell usable (no app list, no forwards).
- If an app port is already in use locally, skip that forward and note it in the summary.
- If app status cannot be determined, show status as "unknown".

## Success Metrics
- Startup completes in <2 seconds on a typical host after first run.
- At least 90% of sessions show an app list and URL on first prompt.
- Support tickets about "how do I open my app" drop after release.

## Open Questions
- Should "vibe" be the only visible command, or should "run" appear in help as a deprecated alias?
- Should we forward only running apps or all apps with containers present?
- Should URL output include only localhost URLs or also public URLs when proxy is configured?
