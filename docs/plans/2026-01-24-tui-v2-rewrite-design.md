# Viberun TUI v2 Rewrite Design

Date: 2026-01-24
Owner: TBD
Status: Approved

## Summary
Rewrite the viberun shell-style TUI on the Charm v2 stack, using Crush as the gold-standard reference for architecture and best practices, while preserving all user-visible behavior and output. This is a destructive internal refactor with strict parity for UX and command behavior.

## Goals
- Migrate to `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and `charm.land/lipgloss/v2`.
- Preserve all user-visible UX behavior, command flows, and output text.
- Align internal architecture with Crush’s `internal/ui` model, components, and styles.
- Centralize styling and capability detection.
- Reduce bugs by adopting Crush patterns for Update/IO separation and component rendering.

## Non-Goals
- Changing CLI command set, flags, or semantics.
- Adding a dashboard or non-shell UI.
- Protocol changes to host/server behavior.

## UX Parity Requirements
Strict parity for:
- Shell REPL behavior (prompt, scrollback, command history, key handling).
- All interactive prompts (setup, wipe, proxy auth, password).
- All non-interactive outputs (help, version, errors, status).

Permitted differences:
- Minor formatting adjustments forced by v2 APIs (e.g., spacing/alignments), as long as copy and behavior are identical.

## Architecture (Crush-style)
### Package layout
- `internal/ui/model/`: single Bubble Tea v2 model owning state and Update routing.
- `internal/ui/components/`: dumb renderers for prompt, header/banner, output, status, help, spinner/progress.
- `internal/ui/styles/`: semantic theme and styles injected into components (no ad-hoc styling).
- `internal/ui/render/`: helpers for tables, aligned columns, help sections.
- `internal/ui/prompts/`: prompt flows using v2 components, preserving UX.

### Core principles (from Crush)
- IO in commands, state mutations in Update, render-only View functions.
- Semantic styles only; no ad-hoc `lipgloss.NewStyle` in feature code.
- Capability detection via `tea.RequestTerminalVersion`, `tea.EnvMsg`, and `tea.KeyboardEnhancementsMsg`.
- Terminal-safe fallback for `NO_COLOR` and `TERM=dumb`.

## Terminal Capabilities
- Request terminal version on Init and gate enhancements accordingly.
- Use `colorprofile` to set correct output color profile.
- Maintain plain-text rendering for non-TTY and NO_COLOR.

## Prompts
- Replace v1 Huh usage with maintained v2 components where possible.
- Preserve prompt text, validation, and flow (including wipe confirmation steps).

## Rendering
- Use `lipgloss/v2` for layout and `bubbles/v2` for keymaps, help, input.
- Recreate current help output and status tables via render helpers.
- Use components that return strings only, allowing deterministic tests.

## Testing
- Unit tests for render helpers (help output, status tables, banners).
- Model state transition tests (key handling, command dispatch).
- PTY integration tests for setup/wipe/attach flows.
- Golden tests for user-visible output to enforce parity.

## Rollout
- No feature flag; replace existing shell implementation directly.
- Remove legacy v1 UI code and dependencies once parity is verified.

## References
- Crush v2 UI model: `~/code/crush/internal/ui/model/ui.go`
- Crush styles: `~/code/crush/internal/ui/styles/`
- Crush capability handling: `~/code/crush/internal/ui/model/ui.go`, `~/code/crush/internal/cmd/root.go`
