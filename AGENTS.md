# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: Go entrypoints (`viberun`, `viberun-server`). Main CLI and host-side server logic live here.
- `internal/`: Core packages (config, server state, SSH args, target parsing, TUI helpers).
- `bin/`: Helper scripts for installs, integration/E2E flows, and container utilities (e.g., `viberun-install`, `viberun-integration`, `vrctl`).
- `skills/`: Codex skill definitions used inside containers.
- `config/`: Shell/TMUX/Starship config and runtime assets (e.g., terminfo).
- `Dockerfile`: Base container image definition.

## Build, Test, and Development Commands
- `mise install`: Install pinned toolchain versions.
- `mise exec -- go build ./cmd/viberun`: Build the CLI.
- `mise exec -- go build ./cmd/viberun-server`: Build the host server.
- `mise exec -- go test ./...`: Run all Go tests.
- `mise exec -- go vet ./...`: Static analysis.
- `docker build -t viberun .`: Build the container image.
- `bin/viberun-e2e-local`: Local E2E flow (Docker + SSH).
- `bin/viberun-integration`: Integration checks against a host.

## Coding Style & Naming Conventions
- Go code is formatted with `gofmt` (tabs for indentation).
- Package names are lower-case; exported identifiers use `CamelCase`.
- Shell scripts in `bin/` should be POSIX‑ish and include `set -e` where appropriate.
- Prefer small, testable helpers for logic that can be unit tested (e.g., argument builders, parsing).

## Testing Guidelines
- Tests use Go’s standard testing framework.
- Run all tests with `go test ./...`; coverage can be measured via `go test ./... -coverprofile=coverage.out`.
- Test files follow `*_test.go` naming. Favor unit tests for pure logic and small helpers; integration tests live in `bin/`.

## Commit & Pull Request Guidelines
- Commit messages use a short scope prefix (e.g., `server: ...`, `client: ...`, `bootstrap: ...`).
- Use imperative, single-line subjects.
- PRs should include: summary of changes, tests run, and risk areas. Link relevant issues if applicable.

## Agent & Skills Notes
- Skills in `skills/` are baked into the container image. If you change skill behavior, rebuild the image and re‑bootstrap the host.

## CLI Styling Guidelines
- Use Charmbracelet `lipgloss` for CLI styling when output is a TTY; preserve plain text when `NO_COLOR` is set or `TERM=dumb`.
- Keep styling minimal and legible in both light and dark themes; prefer `AdaptiveColor` and avoid heavy color blocks.
- Use brand fuchsia sparingly for headers; use a distinct, subdued color for commands and links.
- Keep commands copy‑paste friendly; if adding descriptions, prefer inline `# comment` so pasted lines still work.

## Wipe Command (Safety)
- `viberun wipe [<host>]` deletes local config and wipes host state (containers, images, and viberun data/config/binaries). It uses a TUI confirmation that requires typing `WIPE`.
- Keep this description updated whenever wipe behavior changes (added/removed paths, images, container patterns, or confirmation flow).
