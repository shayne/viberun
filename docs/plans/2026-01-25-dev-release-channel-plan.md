# Dev Release Channel Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the nightly channel with a unified dev-release channel that publishes on every `main` commit and works via `npx viberun@dev` + `uvx viberun-dev`, while keeping prod releases unchanged.

**Architecture:** Add a dev-channel detector in the client, use it to set bootstrap defaults and update prompts, publish dev artifacts/tagged images from CI, and add a separate PyPI package name for dev builds. The server will honor a configurable image reference for update checks.

**Tech Stack:** Go (CLI + server), GitHub Actions, npm, PyPI, bash scripts.

---

### Task 1: Dev version parsing + channel detection (client)

**Files:**
- Modify: `cmd/viberun/main_test.go`
- Modify: `cmd/viberun/main.go`

**Step 1: Write the failing tests**

Add tests for dev channel detection and dev version parsing.

```go
func TestIsDevChannelVersion(t *testing.T) {
	cases := map[string]bool{
		"0.0.0-dev.20260125123045+abc123": true,
		"dev-abc123":                         true,
		"v0.1.0":                              false,
		"dev":                                false,
		"":                                   false,
	}
	for input, expected := range cases {
		if got := isDevChannelVersion(input); got != expected {
			t.Fatalf("isDevChannelVersion(%q)=%v want %v", input, got, expected)
		}
	}
}

func TestParseDevVersionTimestamp(t *testing.T) {
	cases := map[string]struct {
		value   string
		want    int64
		wantOK  bool
	}{
		{"0.0.0-dev.20260125123045+abc123", 20260125123045, true},
		{"0.0.0-dev.20260125123045", 20260125123045, true},
		{"dev-abc123", 0, false},
		{"v0.2.0", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseDevVersionTimestamp(tc.value)
		if ok != tc.wantOK || got != tc.want {
			t.Fatalf("parseDevVersionTimestamp(%q)=%d,%v want %d,%v", tc.value, got, ok, tc.want, tc.wantOK)
		}
	}
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/viberun -run "TestIsDevChannelVersion|TestParseDevVersionTimestamp" -v`

Expected: FAIL with “undefined” function errors.

**Step 3: Write minimal implementation**

Implement helpers in `cmd/viberun/main.go`:
- `isDevChannelVersion(value string) bool`
- `parseDevVersionTimestamp(value string) (int64, bool)`
- `isDevChannel() bool` (uses env `VIBERUN_CHANNEL=dev` or version string)

Ensure the dev detector recognizes the `-dev.<timestamp>` pattern and allows a `dev-<sha>` fallback (no timestamp).

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/viberun -run "TestIsDevChannelVersion|TestParseDevVersionTimestamp" -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/main.go cmd/viberun/main_test.go
git commit -m "client: add dev channel version parsing"
```

---

### Task 2: Separate local dev mode from dev channel + update prompt logic

**Files:**
- Modify: `cmd/viberun/main.go`
- Modify: `cmd/viberun/shell.go`
- Modify: `cmd/viberun/main_test.go`

**Step 1: Write the failing tests**

Add tests for local dev detection helpers and dev update decision logic (pure function).

```go
func TestIsLocalDevVersion(t *testing.T) {
	cases := map[string]bool{
		"dev": true,
		"":    true,
		"0.0.0-dev.20260125123045+abc": false,
		"v0.1.0": false,
	}
	for input, expected := range cases {
		if got := isLocalDevVersion(input); got != expected {
			t.Fatalf("isLocalDevVersion(%q)=%v want %v", input, got, expected)
		}
	}
}

func TestDevUpdateDecision(t *testing.T) {
	local := "0.0.0-dev.20260125123045+abc"
	older := "0.0.0-dev.20260125010101+def"
	newer := "0.0.0-dev.20260126010101+ghi"
	if update, defYes := devUpdateDecision(local, older); !update || !defYes {
		t.Fatalf("expected update with default yes when local newer")
	}
	if update, _ := devUpdateDecision(local, newer); update {
		t.Fatalf("expected no update when remote newer")
	}
	if update, defYes := devUpdateDecision(local, "unknown"); !update || !defYes {
		t.Fatalf("expected update default yes on unknown dev version")
	}
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/viberun -run "TestIsLocalDevVersion|TestDevUpdateDecision" -v`

Expected: FAIL with “undefined” function errors.

**Step 3: Write minimal implementation**

In `cmd/viberun/main.go`:
- Add `isLocalDevVersion(value string) bool` and update `isDevMode()` (in `cmd/viberun/shell.go`) to use **local dev only**: `VIBERUN_DEV` or `isDevRun()` or `isLocalDevVersion(version)`.
- Update all local-dev code paths (`ensureDevServerSynced`, `stageLocalImage`, proxy dev tag logic, `runRemoteAppUpdate`, etc.) to use a new `isLocalDevMode()` helper.
- Add a pure helper `devUpdateDecision(local, remote string) (update bool, defaultYes bool)` that uses `compareDevVersions` (timestamp compare) and defaults to **Yes** on parse failure.
- Use that helper in `runShellSetup` when `isDevChannel()` is true to choose default prompt and update decision.

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/viberun -run "TestIsLocalDevVersion|TestDevUpdateDecision" -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/main.go cmd/viberun/shell.go cmd/viberun/main_test.go
git commit -m "client: split local dev from dev channel updates"
```

---

### Task 3: Dev channel bootstrap defaults + gateway env

**Files:**
- Modify: `cmd/viberun/shell_setup.go`
- Modify: `cmd/viberun/gateway.go`
- Modify: `cmd/viberun/shell_setup.go`
- Modify: `cmd/viberun/shell_startup.go`
- Modify: `cmd/viberun/shell_actions.go`
- Modify: `cmd/viberun/shell_commands.go`
- Modify: `cmd/viberun/wipe_flow.go`
- Modify: `cmd/viberun/shell_gateway.go`

**Step 1: Write a failing test (lightweight)**

Add a small test for `devChannelEnv()` (pure helper) in `cmd/viberun/main_test.go`.

```go
func TestDevChannelEnvDefaults(t *testing.T) {
	origVersion := version
	version = "0.0.0-dev.20260125123045+abc"
	t.Cleanup(func() { version = origVersion })
	if got := devChannelEnv(); got["VIBERUN_SERVER_VERSION"] != "dev" {
		t.Fatalf("expected VIBERUN_SERVER_VERSION=dev, got %q", got["VIBERUN_SERVER_VERSION"])
	}
	if got["VIBERUN_IMAGE"] == "" || got["VIBERUN_PROXY_IMAGE"] == "" {
		t.Fatalf("expected dev image defaults")
	}
}
```

**Step 2: Run test to verify failure**

Run: `go test ./cmd/viberun -run TestDevChannelEnvDefaults -v`

Expected: FAIL (undefined helper).

**Step 3: Implement helper + wire env**

- Add `devChannelEnv()` in `cmd/viberun/main.go` to provide:
  - `VIBERUN_SERVER_VERSION=dev`
  - `VIBERUN_IMAGE=ghcr.io/<repo>/viberun:dev`
  - `VIBERUN_PROXY_IMAGE=ghcr.io/<repo>/viberun-proxy:dev`
  - respect user overrides from the process environment
- In `cmd/viberun/shell_setup.go`, append dev env defaults to the bootstrap env when `isDevChannel()` is true.
- In all `startGateway(...)` call sites, pass `devChannelEnv()` as `extraEnv` (or `nil` when not dev).

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/viberun -run TestDevChannelEnvDefaults -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/main.go cmd/viberun/main_test.go cmd/viberun/shell_setup.go cmd/viberun/gateway.go cmd/viberun/shell_startup.go cmd/viberun/shell_actions.go cmd/viberun/shell_commands.go cmd/viberun/wipe_flow.go cmd/viberun/shell_gateway.go
git commit -m "client: apply dev channel env defaults"
```

---

### Task 4: Server image override for update checks

**Files:**
- Modify: `cmd/viberun-server/main.go`
- Modify: `cmd/viberun-server/update_check.go`
- Modify: `cmd/viberun-server/main_test.go`

**Step 1: Write the failing test**

```go
func TestDefaultImageRefUsesEnv(t *testing.T) {
	t.Setenv("VIBERUN_IMAGE", "ghcr.io/shayne/viberun/viberun:dev")
	if got := defaultImageRef(); got != "ghcr.io/shayne/viberun/viberun:dev" {
		t.Fatalf("defaultImageRef=%q want env override", got)
	}
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./cmd/viberun-server -run TestDefaultImageRefUsesEnv -v`

Expected: FAIL (undefined helper).

**Step 3: Implement minimal code**

- Add `defaultImageRef()` in `cmd/viberun-server/main.go` to return `VIBERUN_IMAGE` when set, else `viberun:latest`.
- Replace references to `defaultImage` with `defaultImageRef()` in `main.go` and `update_check.go`.

**Step 4: Run tests to verify pass**

Run: `go test ./cmd/viberun-server -run TestDefaultImageRefUsesEnv -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun-server/main.go cmd/viberun-server/update_check.go cmd/viberun-server/main_test.go
git commit -m "server: honor VIBERUN_IMAGE for updates"
```

---

### Task 5: PyPI dev package support in build scripts

**Files:**
- Modify: `tools/packaging/build-pypi.sh`
- Modify: `packaging/pypi/pyproject.toml`
- Modify: `packaging/pypi/viberun/__init__.py`

**Step 1: Write a failing test (script-level sanity)**

Add a tiny shell test script under `tools/packaging` or extend an existing task to build with `PYPI_PACKAGE_NAME` and assert the sdist/wheel metadata includes `viberun-dev`. (We can use `python -m zipfile -l` on the wheel and grep `METADATA`.)

**Step 2: Run the test to verify failure**

Run (example):

```bash
VERSION=0.0.0-dev.20260125123045+abc PYPI_PACKAGE_NAME=viberun-dev PYPI_VERSION=0.0.0.dev20260125123045 tools/packaging/build-pypi.sh
python3 - <<'PY'
import zipfile, glob
wheel = glob.glob('dist/pypi/*.whl')[0]
with zipfile.ZipFile(wheel) as z:
    meta = [n for n in z.namelist() if n.endswith('METADATA')][0]
    data = z.read(meta).decode()
    assert 'Name: viberun-dev' in data
PY
```

Expected: FAIL until the build script updates metadata.

**Step 3: Implement minimal script updates**

- In `tools/packaging/build-pypi.sh`, accept:
  - `PYPI_PACKAGE_NAME` (default `viberun`)
  - `PYPI_VERSION` (default `VERSION#v`)
- Update the staging `pyproject.toml` to replace the `name = "..."` field with `PYPI_PACKAGE_NAME`.
- Update `packaging/pypi/viberun/__init__.py` to use a `DIST_NAME` constant; replace it during staging with `PYPI_PACKAGE_NAME`.

**Step 4: Run the test to verify pass**

Re-run the script + wheel metadata check; expect success.

**Step 5: Commit**

```bash
git add tools/packaging/build-pypi.sh packaging/pypi/pyproject.toml packaging/pypi/viberun/__init__.py
git commit -m "packaging: support viberun-dev PyPI builds"
```

---

### Task 6: Rename nightly workflow to dev + publish dev artifacts

**Files:**
- Modify: `.github/workflows/nightly.yml` (rename to `dev.yml`)
- Modify: `.github/workflows/release.yml` (only if needed for shared tasks)

**Step 1: Write a failing test (workflow lint check)**

Add a simple CI-only check locally by running `yq`/`python -m yaml` to validate the new workflow syntax. (If not available, skip.)

**Step 2: Implement workflow changes**

- Rename workflow to “Dev Release”.
- Remove `schedule` trigger; keep `push` on `main` and `workflow_dispatch`.
- Change tag/release names from `nightly` → `dev` and `Nightly` → `Dev`.
- Build version string like:
  - `VERSION=0.0.0-dev.<UTC_TIMESTAMP>+<shortsha>`
- Add package build/publish steps (from `release.yml`):
  - Build vendor + packages
  - npm publish with `--tag dev`
  - PyPI publish with `PYPI_PACKAGE_NAME=viberun-dev` and `PYPI_VERSION=0.0.0.dev<UTC_TIMESTAMP>`
- Push container images tagged `:dev` (do not update `:latest`).

**Step 3: Validate workflow syntax**

Run a YAML syntax check if tool available.

**Step 4: Commit**

```bash
git add .github/workflows/dev.yml
git commit -m "ci: publish dev channel from main"
```

---

### Task 7: Update install script + README to dev channel

**Files:**
- Modify: `install.sh`
- Modify: `README.md`

**Step 1: Update docs**

- Replace `--nightly` with `--dev`.
- Replace download paths `/download/nightly/...` with `/download/dev/...`.
- Add quickstart commands for `npx viberun@dev` and `uvx viberun-dev`.

**Step 2: Manual check**

Run: `rg -n "nightly" -S` to ensure no remaining references.

**Step 3: Commit**

```bash
git add install.sh README.md
git commit -m "docs: rename nightly channel to dev"
```

---

### Task 8: Full test pass

**Step 1: Run unit tests**

Run: `go test ./...`

Expected: PASS.

**Step 2: (Optional) Package sanity**

Run dev PyPI build check from Task 5 once more if needed.

**Step 3: Commit (if changes)**

Only if any changes were made during test fixes.

---

Plan complete and saved to `docs/plans/2026-01-25-dev-release-channel-plan.md`.

Two execution options:

1. Subagent-Driven (this session) - I dispatch fresh subagent per task, review between tasks, fast iteration
2. Parallel Session (separate) - Open new session with executing-plans, batch execution with checkpoints

Which approach?
