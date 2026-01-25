# Clickable tmux bar buttons (shell + detach)

Date: 2026-01-25

## Summary
Add mouse-clickable buttons to the tmux status bar inside the container. The bar will expose a one-time `shell` button that creates a `shell` window and immediately switches to it, then becomes a `shell` tab for returning. A clickable agent tab (labelled by the agent window name such as `codex`) provides a mouse-friendly way to switch back. A bold `detach` button appears on the far right.

## Goals
- Provide a mouse-only workflow to create a shell window and switch between agent and shell.
- Keep the existing URL/status section intact.
- Preserve default window-list clicking in the center of the status bar.

## Non-goals
- Re-style the full tmux status theme beyond adding button-like labels.
- Introduce new tmux plugins or external dependencies.

## UX details
- Left side layout: `viberun <app> <agent tab> <shell button|shell tab>`.
- Right side layout: `<status + url> <detach button>`.
- The `shell` button is shown only when a shell window does not exist.
- Clicking `shell` creates a new `shell` window and switches to it immediately.
- After creation, the `shell` label becomes a tab that switches back to the shell window.
- Clicking the agent tab switches back to the agent window.

## Implementation plan

### tmux configuration
- Enable mouse support so status-line click bindings fire:
  - `set -g mouse on`
- Add a global mouse binding on the status line that routes clicks based on `#{mouse_status_range}`.
  - If the range is empty, fall back to `select-window -t {mouse}` so the normal window list still works.
  - If range is `shell`, create or select the `shell` window and switch to it.
  - If range is `detach`, run `detach-client`.
  - Otherwise, interpret the range value as a window name and select it.

### Status bar rendering (`bin/viberun-tmux-status`)
- Wrap the agent label in a user range using its window name:
  - `#[range=user|<agent>]...#[range=default]`
- If a `shell` window exists, render `shell` as a tab with `range=user|shell`.
- If it does not exist, render a `shell` button with the same range so clicks are routed consistently.
- Append a bold `detach` label on the right wrapped in `range=user|detach`.
- Detect shell window existence using tmux:
  - `tmux list-windows -F '#{window_name}'` and check for `shell`.

### Styling
- Reuse the existing status colors in `bin/viberun-tmux-status`.
- Make `detach` bold and optionally use a subtle foreground color to read as a button.
- Keep the layout readable at `status-left-length=60` and `status-right-length=80`.

## Error handling
- If tmux queries fail inside the status script, default to showing the `shell` button.
- If `select-window` fails for a named range, fall back to `select-window -t {mouse}`.

## Testing plan
- Manual: start a container session, verify click targets:
  - Click agent tab -> switches to agent window.
  - Click `shell` button -> creates window, switches to shell, button becomes tab.
  - Click `shell` tab -> switches to shell.
  - Click `detach` -> detaches client.
  - Click a window name in the window list -> switches windows as before.
- Manual: verify `shell` button disappears after shell window exists.

## Rollout notes
- This is container-only tmux config; no host-side changes required.
- If tmux mouse behavior is undesired, we can disable with `set -g mouse off`.
