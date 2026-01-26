# Branch Environments Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add branch environments with `vibe <app> --branch <branch>`, branch listing/removal/apply, and host-side clone/apply workflows that keep `data/` isolated while promoting `app/` changes back to the base app.

**Architecture:** Branch envs are derived app names (`<app>--<branch>`) with metadata stored under the branch app’s volume. Branch creation clones the base app’s home snapshot into a new home volume. Apply is host-driven: snapshot base, use a shadow git repo to merge base+branch (conflicts surface in branch worktree), then sync `app/` from branch to base. Host RPC exposes branch create/apply to allow agent-driven workflows.

**Tech Stack:** Go (server + shell), btrfs tools, docker, git (host), shell scripts (`vrctl`), Bubble Tea/Lipgloss for CLI help.

---

### Task 1: Add branch name helpers (shared)

**Files:**
- Create: `internal/branch/branch.go`
- Create: `internal/branch/branch_test.go`

**Step 1: Write failing tests**

```go
package branch

import "testing"

func TestDerivedAppName(t *testing.T) {
	got, err := DerivedAppName("company", "contact-form")
	if err != nil {
		t.Fatalf("DerivedAppName error: %v", err)
	}
	if got != "company--contact-form" {
		t.Fatalf("got %q", got)
	}
}

func TestDerivedAppNameRejectsLong(t *testing.T) {
	_, err := DerivedAppName("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "b")
	if err == nil {
		t.Fatalf("expected error for too-long derived name")
	}
}

func TestNormalizeBranchName(t *testing.T) {
	got, err := NormalizeBranchName("Contact-Form")
	if err != nil {
		t.Fatalf("NormalizeBranchName error: %v", err)
	}
	if got != "contact-form" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBranchNameRejectsInvalid(t *testing.T) {
	_, err := NormalizeBranchName("bad/branch")
	if err == nil {
		t.Fatalf("expected error for invalid branch")
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./internal/branch -v`

Expected: FAIL (missing package/functions).

**Step 3: Implement helpers**

```go
package branch

import (
	"fmt"

	"github.com/shayne/viberun/internal/proxy"
)

const maxAppLength = 63

func NormalizeBranchName(raw string) (string, error) {
	return proxy.NormalizeAppName(raw)
}

func DerivedAppName(base string, branch string) (string, error) {
	baseNorm, err := proxy.NormalizeAppName(base)
	if err != nil {
		return "", err
	}
	branchNorm, err := NormalizeBranchName(branch)
	if err != nil {
		return "", err
	}
	derived := baseNorm + "--" + branchNorm
	if len(derived) > maxAppLength {
		return "", fmt.Errorf("branch app name is too long")
	}
	return derived, nil
}
```

**Step 4: Re-run tests**

Run: `go test ./internal/branch -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/branch/branch.go internal/branch/branch_test.go
git commit -m "server: add branch naming helpers"
```

---

### Task 2: Branch metadata helpers + list formatting (server)

**Files:**
- Create: `cmd/viberun-server/branch_meta.go`
- Create: `cmd/viberun-server/branch_meta_test.go`

**Step 1: Write failing tests**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBranchMetaLoadAndList(t *testing.T) {
	base := t.TempDir()
	meta := branchMeta{BaseApp: "app", Branch: "feature", ShadowRepo: "/tmp/repo", BaseSnapshotRef: "branch-base-1"}
	if err := writeBranchMetaAt(base, "app--feature", meta); err != nil {
		t.Fatalf("writeBranchMetaAt: %v", err)
	}
	list, err := listBranchMetasAt(base, "app")
	if err != nil {
		t.Fatalf("listBranchMetasAt: %v", err)
	}
	if len(list) != 1 || list[0].Branch != "feature" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(base, "app--feature", "branch.json")); err != nil {
		t.Fatalf("missing branch.json: %v", err)
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./cmd/viberun-server -run TestBranchMetaLoadAndList -v`

Expected: FAIL (missing helpers).

**Step 3: Implement metadata helpers**

```go
const branchMetaFilename = "branch.json"

var shadowGitBaseDir = "/var/lib/viberun/git"

type branchMeta struct {
	BaseApp         string    `json:"base_app"`
	Branch          string    `json:"branch"`
	CreatedAt       time.Time `json:"created_at"`
	BaseSnapshotRef string    `json:"base_snapshot_ref"`
	ShadowRepo      string    `json:"shadow_repo"`
}

func branchMetaPathAt(baseDir, app string) string {
	return filepath.Join(baseDir, sanitizeHostRPCName(app), branchMetaFilename)
}

func branchMetaPath(app string) string {
	return branchMetaPathAt(homeVolumeBaseDir, app)
}

func writeBranchMetaAt(baseDir, app string, meta branchMeta) error {
	path := branchMetaPathAt(baseDir, app)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readBranchMetaAt(baseDir, app string) (branchMeta, bool, error) {
	path := branchMetaPathAt(baseDir, app)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return branchMeta{}, false, nil
		}
		return branchMeta{}, false, err
	}
	var meta branchMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return branchMeta{}, false, err
	}
	return meta, true, nil
}

func listBranchMetasAt(baseDir, baseApp string) ([]branchMeta, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []branchMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, ok, err := readBranchMetaAt(baseDir, entry.Name())
		if err != nil {
			return nil, err
		}
		if !ok || meta.BaseApp != baseApp {
			continue
		}
		out = append(out, meta)
	}
	return out, nil
}
```

**Step 4: Re-run tests**

Run: `go test ./cmd/viberun-server -run TestBranchMetaLoadAndList -v`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/viberun-server/branch_meta.go cmd/viberun-server/branch_meta_test.go
git commit -m "server: add branch metadata helpers"
```

---

### Task 3: Branch create (clone base volume + metadata)

**Files:**
- Modify: `cmd/viberun-server/main.go`
- Create: `cmd/viberun-server/branch_create.go`
- Create: `cmd/viberun-server/branch_create_test.go`

**Step 1: Write failing tests (validation + derived name)**

```go
package main

import "testing"

func TestValidateBranchCreateArgs(t *testing.T) {
	app, branch, derived, err := validateBranchCreateArgs("myapp", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != "myapp" || branch != "feature" || derived != "myapp--feature" {
		t.Fatalf("unexpected values: %q %q %q", app, branch, derived)
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./cmd/viberun-server -run TestValidateBranchCreateArgs -v`

Expected: FAIL (missing function).

**Step 3: Implement branch create**

```go
func validateBranchCreateArgs(base string, branch string) (string, string, string, error) {
	baseNorm, err := proxy.NormalizeAppName(base)
	if err != nil {
		return "", "", "", err
	}
	branchNorm, err := branchpkg.NormalizeBranchName(branch)
	if err != nil {
		return "", "", "", err
	}
	derived, err := branchpkg.DerivedAppName(baseNorm, branchNorm)
	if err != nil {
		return "", "", "", err
	}
	return baseNorm, branchNorm, derived, nil
}

func createBranchEnv(base string, branch string) (branchMeta, error) {
	baseApp, branchName, derived, err := validateBranchCreateArgs(base, branch)
	if err != nil {
		return branchMeta{}, err
	}
	if exists, _ := containerExists("viberun-" + derived); exists {
		return branchMeta{}, fmt.Errorf("branch already exists: %s", derived)
	}
	baseCfg, ok, err := ensureHomeVolume(baseApp, false)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("base app volume does not exist")
		}
		return branchMeta{}, err
	}
	baseContainer := "viberun-" + baseApp
	if exists, _ := containerExists(baseContainer); !exists {
		return branchMeta{}, fmt.Errorf("base app container does not exist")
	}
	stamp := time.Now().UTC().Format("20060102150405")
	tag := fmt.Sprintf("branch-base-%s", stamp)
	if err := snapshotContainer(baseContainer, baseCfg, tag); err != nil {
		return branchMeta{}, err
	}
	branchCfg, _, err := ensureHomeVolume(derived, true)
	if err != nil {
		return branchMeta{}, err
	}
	snapshotPath := snapshotPathForTag(baseCfg, tag)
	if err := copySnapshotToHome(snapshotPath, branchCfg.MountDir); err != nil {
		return branchMeta{}, err
	}
	meta := branchMeta{
		BaseApp:         baseApp,
		Branch:          branchName,
		CreatedAt:       time.Now().UTC(),
		BaseSnapshotRef: tag,
		ShadowRepo:      filepath.Join(shadowGitBaseDir, baseApp+".git"),
	}
	if err := writeBranchMetaAt(homeVolumeBaseDir, derived, meta); err != nil {
		return branchMeta{}, err
	}
	return meta, nil
}

func copySnapshotToHome(snapshotPath string, destHome string) error {
	if strings.TrimSpace(snapshotPath) == "" || strings.TrimSpace(destHome) == "" {
		return fmt.Errorf("snapshot and dest are required")
	}
	if err := os.RemoveAll(destHome); err != nil {
		return err
	}
	if err := os.MkdirAll(destHome, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("tar", "-C", snapshotPath, "-cf", "-", ".")
	cmd2 := exec.Command("tar", "-C", destHome, "-xf", "-")
	r, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd2.Stdin = r
	if err := cmd2.Start(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return cmd2.Wait()
}
```

**Step 4: Re-run tests**

Run: `go test ./cmd/viberun-server -run TestValidateBranchCreateArgs -v`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/viberun-server/branch_create.go cmd/viberun-server/branch_create_test.go cmd/viberun-server/main.go
git commit -m "server: add branch env creation"
```

---

### Task 4: Branch list/delete/apply CLI + apply engine (host)

**Files:**
- Modify: `cmd/viberun-server/main.go`
- Create: `cmd/viberun-server/branch_command.go`
- Create: `cmd/viberun-server/branch_apply.go`
- Create: `cmd/viberun-server/branch_command_test.go`

**Step 1: Write failing tests (command parse)**

```go
package main

import "testing"

func TestParseBranchCommand(t *testing.T) {
	cmd, err := parseBranchCommand([]string{"list", "myapp"})
	if err != nil {
		t.Fatalf("parseBranchCommand error: %v", err)
	}
	if cmd.action != "list" || cmd.base != "myapp" {
		t.Fatalf("unexpected: %+v", cmd)
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./cmd/viberun-server -run TestParseBranchCommand -v`

Expected: FAIL

**Step 3: Implement branch command + apply engine**

```go
type branchCommand struct {
	action string
	base   string
	branch string
}

func parseBranchCommand(args []string) (branchCommand, error) {
	if len(args) < 2 {
		return branchCommand{}, newUsageError("Usage: viberun-server branch <list|create|delete|apply> <app> [branch]")
	}
	return branchCommand{action: args[0], base: args[1], branch: strings.TrimSpace(strings.Join(args[2:], " "))}, nil
}

func handleBranchCommand(args []string) error {
	cmd, err := parseBranchCommand(args)
	if err != nil {
		return err
	}
	switch cmd.action {
	case "list":
		return runBranchList(cmd.base)
	case "create":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch create <app> <branch>")
		}
		meta, err := createBranchEnv(cmd.base, cmd.branch)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Created branch %s for %s\n", meta.Branch, meta.BaseApp)
		return nil
	case "delete", "rm":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch delete <app> <branch>")
		}
		return deleteBranchEnv(cmd.base, cmd.branch)
	case "apply":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch apply <app> <branch>")
		}
		return applyBranch(cmd.base, cmd.branch)
	default:
		return newUsageError("Usage: viberun-server branch <list|create|delete|apply> <app> [branch]")
	}
}
```

Implement apply engine (host git + snapshot + sync):

```go
func applyBranch(base string, branch string) error {
	baseApp, branchName, derived, err := validateBranchCreateArgs(base, branch)
	if err != nil {
		return err
	}
	meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, derived)
	if err != nil || !ok {
		return fmt.Errorf("branch not found: %s", branchName)
	}
	baseCfg, ok, err := ensureHomeVolume(baseApp, false)
	if err != nil || !ok {
		return fmt.Errorf("base app volume missing")
	}
	branchCfg, ok, err := ensureHomeVolume(derived, false)
	if err != nil || !ok {
		return fmt.Errorf("branch volume missing")
	}
	baseContainer := "viberun-" + baseApp
	if exists, _ := containerExists(baseContainer); !exists {
		return fmt.Errorf("base app container missing")
	}
	if _, err := createSnapshot(baseContainer, baseApp); err != nil {
		return err
	}
	repo := meta.ShadowRepo
	if err := ensureShadowRepo(repo); err != nil {
		return err
	}
	if err := gitCommitWorktree(repo, "main", filepath.Join(baseCfg.MountDir, "app")); err != nil {
		return err
	}
	if err := gitCommitWorktree(repo, meta.Branch, filepath.Join(branchCfg.MountDir, "app")); err != nil {
		return err
	}
	if err := gitMergeIntoBranch(repo, meta.Branch, "main", filepath.Join(branchCfg.MountDir, "app")); err != nil {
		return err
	}
	if err := gitFastForward(repo, "main", meta.Branch); err != nil {
		return err
	}
	return syncAppDir(filepath.Join(branchCfg.MountDir, "app"), filepath.Join(baseCfg.MountDir, "app"))
}
```

**Step 4: Re-run tests**

Run: `go test ./cmd/viberun-server -run TestParseBranchCommand -v`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/viberun-server/branch_command.go cmd/viberun-server/branch_command_test.go cmd/viberun-server/branch_apply.go cmd/viberun-server/main.go
git commit -m "server: add branch list/delete/apply"
```

---

### Task 5: Host RPC endpoints + vrctl updates

**Files:**
- Modify: `cmd/viberun-server/hostrpc.go`
- Modify: `cmd/viberun-server/hostrpc_test.go`
- Modify: `bin/vrctl`
- Modify: `Dockerfile`
- Create: `bin/viberun-apply`

**Step 1: Write failing tests (host RPC apply/create)**

```go
func TestHostRPCBranchApply(t *testing.T) {
	server := &hostRPCServer{
		token: "token",
		app:   "app--feature",
		branchApplyFn: func(app string) error { return nil },
	}
	server.httpServer = &http.Server{Handler: server.routes()}
	req := httptest.NewRequest(http.MethodPost, "http://unix/branch/apply", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	server.routes().ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Result().StatusCode)
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./cmd/viberun-server -run TestHostRPCBranchApply -v`

Expected: FAIL

**Step 3: Implement endpoints + vrctl wrapper**

- Add to `hostRPCServer`:

```go
branchCreateFn func(base string, branch string) (branchMeta, error)
branchApplyFn  func(app string) error
```

- Add routes in `routes()`:

```go
mux.HandleFunc("/branch/create", s.handleBranchCreate)
mux.HandleFunc("/branch/apply", s.handleBranchApply)
```

- Implement handlers using body payloads (branch name as plain text).

- Update `startHostRPC` to set `branchCreateFn: createBranchEnv` and `branchApplyFn: applyBranchForApp`.

- Add `applyBranchForApp(app string) error` that loads `branch.json`, then calls `applyBranch(meta.BaseApp, meta.Branch)`.

- Update `bin/vrctl` usage + commands:

```
vrctl host branch create <name>
vrctl host branch apply
```

- Add `bin/viberun-apply`:

```sh
#!/bin/sh
set -eu
exec vrctl host branch apply
```

- Copy it in `Dockerfile` to `/usr/local/bin/apply`.

**Step 4: Re-run tests**

Run: `go test ./cmd/viberun-server -run TestHostRPCBranchApply -v`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/viberun-server/hostrpc.go cmd/viberun-server/hostrpc_test.go bin/vrctl bin/viberun-apply Dockerfile
git commit -m "server: add branch host rpc and apply wrapper"
```

---

### Task 6: Shell commands for branch envs

**Files:**
- Modify: `cmd/viberun/shell_commands.go`
- Modify: `cmd/viberun/shell_actions.go`
- Modify: `cmd/viberun/shell_registry.go`
- Modify: `cmd/viberun/shell_commands_test.go`

**Step 1: Write failing tests (vibe --branch parsing)**

```go
func TestParseVibeBranchArgs(t *testing.T) {
	app, branch, err := parseVibeArgs([]string{"myapp", "--branch", "feature"})
	if err != nil {
		t.Fatalf("parseVibeArgs error: %v", err)
	}
	if app != "myapp" || branch != "feature" {
		t.Fatalf("unexpected: %q %q", app, branch)
	}
}
```

**Step 2: Run tests to confirm failure**

Run: `go test ./cmd/viberun -run TestParseVibeBranchArgs -v`

Expected: FAIL

**Step 3: Implement `vibe --branch` + branch commands**

- Add `parseVibeArgs(args []string) (app, branch string, err error)`.
- In `dispatchGlobalCommand`, if branch is set:
  - Validate branch/app via `internal/branch` helpers.
  - Run `viberun-server branch create <app> <branch>` via gateway (if derived app doesn’t exist).
  - Attach to derived app via new `actionVibeBranch` (or reuse `actionVibe` with derived app).
- Add app-scope `branch` command that supports `branch`, `branch list`, `branch rm <branch>`, `branch apply <branch>` and executes via `gateway.command`.

**Step 4: Re-run tests**

Run: `go test ./cmd/viberun -run TestParseVibeBranchArgs -v`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/viberun/shell_commands.go cmd/viberun/shell_actions.go cmd/viberun/shell_registry.go cmd/viberun/shell_commands_test.go
git commit -m "client: add branch commands and vibe --branch"
```

---

### Task 7: Update UI help registry + snapshots

**Files:**
- Modify: `internal/ui/model/registry.go`
- Modify: `internal/ui/testdata/help_global.txt`
- Modify: `internal/ui/testdata/help_app.txt`
- Modify: `internal/ui/pty/runner_test.go` (only if output changes unexpectedly)

**Step 1: Update help entries**

- Add app-scope `branch` command help and adjust `vibe` usage to mention `--branch`.

**Step 2: Update snapshots**

- Run `go test ./internal/ui/model -run TestHelp.* -v` and update `help_global.txt` + `help_app.txt` to match output.

**Step 3: Commit**

```bash
git add internal/ui/model/registry.go internal/ui/testdata/help_global.txt internal/ui/testdata/help_app.txt
git commit -m "ui: add branch help entries"
```

---

### Task 8: Full verification

**Step 1: Run full tests**

Run: `go test ./...`

Expected: PASS

**Step 2: Summarize changes + open questions**

Document any remaining TODOs (e.g., branch sync/rebase, custom domains, TTL) in the PR summary.

---

Plan complete and saved to `docs/plans/2026-01-25-branch-envs-plan.md`.

Two execution options:

1. Subagent-Driven (this session) - I dispatch fresh subagent per task, review between tasks, fast iteration
2. Parallel Session (separate) - Open new session with executing-plans, batch execution with checkpoints

Which approach?
