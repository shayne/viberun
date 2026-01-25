# Dev Release Channel Design

Date: 2026-01-25

## Goal

Create a unified dev-release channel that publishes on every `main` commit and can be installed via:

- `npx viberun@dev` (npm dist-tag)
- `uvx viberun-dev` (separate PyPI package)

The dev channel should update the client, server binary, and container images together, and default to updating when a newer dev build exists.

## Non-Goals

- Replace or change production release flow (tagged releases remain unchanged).
- Support arbitrary prerelease selection in uvx (PyPI will use a separate package name instead).

## Architecture Overview

The dev channel is a parallel release track to prod. It reuses the existing release pipeline but publishes to:

- GitHub release tag `dev` (force-updated on each `main` commit).
- Container images `ghcr.io/<repo>/viberun:dev` and `.../viberun-proxy:dev`.
- npm dist-tag `dev` on the existing `viberun` package.
- PyPI package `viberun-dev` with PEP 440 dev versions.

The client detects the dev channel using the embedded version string and switches defaults to dev artifacts. This avoids env flags for end users.

## Components and Responsibilities

1) GitHub Actions
- Rename nightly workflow to dev. Remove schedule; trigger only on `main` pushes.
- Build and publish GH release assets tagged `dev` (prerelease true).
- Push `viberun:dev` and `viberun-proxy:dev` images.
- Publish npm with a dev version + dist-tag `dev`.
- Publish PyPI `viberun-dev` with dev version.

2) Packaging
- npm: same package name `viberun`, version like `0.0.0-dev.<timestamp>+<sha>`.
- PyPI: new package name `viberun-dev`, version like `0.0.0.dev<timestamp>`.
- Vendor binaries still embedded in packages.

3) Client (viberun)
- Add dev-channel detection based on version string.
- When dev channel is active, set defaults:
  - `VIBERUN_SERVER_VERSION=dev`
  - `VIBERUN_IMAGE=ghcr.io/<repo>/viberun:dev`
  - `VIBERUN_PROXY_IMAGE=ghcr.io/<repo>/viberun-proxy:dev`
- Update prompt behavior for dev:
  - Compare dev versions and default Yes when remote is older.
  - If comparison fails, prompt with default Yes in dev channel.

4) Server (viberun-server)
- Update image pulls and update checks to respect dev channel by using `viberun:dev` when configured.
- Keep prod defaults when dev channel is not active.

## Data Flow

1) Dev release build
- `main` push triggers dev workflow.
- Workflow builds binaries, packages, and container images.
- GH release tag `dev` updated with assets.
- npm/pypi publish to dev channels.

2) Client setup
- User runs `npx viberun@dev` or `uvx viberun-dev`.
- Client determines dev channel from version.
- Setup sets dev artifact envs before bootstrap.
- Bootstrap downloads server binary from GH `dev` release and pulls dev images.

3) Update flow
- Client fetches remote server version.
- If dev: compare dev versions, default update when newer.
- Server update checks use `viberun:dev` for image comparisons.

## Error Handling

- Missing dev assets: show a clear error and do not silently fall back to prod tags.
- Image pulls: continue to warn on pull failures (existing behavior), but keep the dev image names.
- Version comparison failure: in dev channel, prompt with default Yes; in prod, default No (existing behavior).

## Testing

- Unit tests for dev version parsing/comparison and dev channel detection.
- CI sanity checks for dev packages:
  - `uvx --from <wheel> viberun --help` for `viberun-dev`.
  - npm publish dry-run or package extraction validation.
- Update README and install script references from nightly to dev.

## Rollout

- Rename nightly docs/flags to dev (`install.sh`, README).
- Publish first dev release from main.
- Validate:
  - `npx viberun@dev --version` shows dev version.
  - `uvx viberun-dev --version` works.
  - Setup pulls dev server and images.

