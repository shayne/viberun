# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: Go entrypoints (`viberun`, `viberun-server`).
- `internal/`: core packages (config, server state, SSH args, target parsing).
- `bin/`: helper scripts (`viberun-e2e-local`, `viberun-integration`, installers).
- `skills/`: Codex skills and templates used inside containers.
- `Dockerfile`: base container image.

## Build, Test, and Development Commands
- `mise install`: installs toolchain pinned by `mise`.
- `mise exec -- go build ./cmd/viberun`: build CLI.
- `mise exec -- go build ./cmd/viberun-server`: build server binary.
- `mise exec -- go test ./...`: run all Go tests.
- `mise exec -- go vet ./...`: static analysis.
- `docker build -t viberun .`: build container image (fallback).
- `bin/viberun-e2e-local`: local E2E flow via SSH.
- `bin/viberun-integration`: integration checks against a host.

## Coding Style & Naming Conventions
- Go code is formatted with `gofmt` (tabs for indentation).
- Package names are lower-case; exported identifiers are `CamelCase`.
- Shell scripts in `bin/` should be POSIX‑ish and include `set -e` where appropriate.

## Testing Guidelines
- Use `go test ./...` for unit tests and `go vet ./...` for linting.
- E2E scripts live in `bin/` and assume Docker + SSH.
- No explicit coverage target is defined—add tests for new logic where feasible.

## Commit & Pull Request Guidelines
- Recent commit history uses a short scope prefix, e.g. `bootstrap: ...`, `client: ...`.
- Prefer imperative, single‑line subjects (example: `server: fix port sync`).
- PRs should describe the change, mention tests run, and call out any risk areas.

## Agent & Skills Notes
- Skills live under `skills/` and are baked into the container image.
- If you change skill behavior, rebuild the image and re‑bootstrap the host.
