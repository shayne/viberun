# Branch Spinner Labels + Apply Auto-Attach Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a status label next to the spinner during branch creation, and auto-attach back to base after applying a branch from a branch tmux.

**Architecture:** Add a lightweight busy label to the shell state and render it in the spinner line; set the label for branch creation commands. For branch apply, extend host RPC to resolve base/branch from metadata when called from a branch app and optionally trigger an attach event back to the base app; update vrctl/apply helpers to request attach.

**Tech Stack:** Go (bubbletea spinner), viberun host RPC (HTTP over unix socket), shell scripts (`bin/vrctl`, `bin/viberun-apply`).

---

### Task 1: Spinner label during branch creation

**Files:**
- Modify: `cmd/viberun/shell.go`
- Modify: `cmd/viberun/shell_model.go`
- Modify: `cmd/viberun/shell_commands.go`
- Test: `cmd/viberun/shell_formatting_test.go`

**Step 1: Write the failing test**

Add to `cmd/viberun/shell_formatting_test.go`:

```go
func TestRenderLoadingLineShowsBusyLabel(t *testing.T) {
	m := shellModel{promptSpin: spinner.New(), state: &shellState{busyLabel: "Creating branch rainbow..."}}
	m.promptSpin.Spinner = spinner.Spinner{Frames: []string{"*"}, FPS: 50 * time.Millisecond}
	if got := renderLoadingLine(m); !strings.Contains(got, "Creating branch rainbow...") {
		t.Fatalf("expected busy label in loading line, got %q", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
mise exec -- go test ./cmd/viberun -run TestRenderLoadingLineShowsBusyLabel
```
Expected: FAIL because `renderLoadingLine` does not include the label yet.

**Step 3: Write minimal implementation**

- Add `busyLabel string` to `shellState` in `cmd/viberun/shell.go`.
- Update `renderLoadingLine` in `cmd/viberun/shell_model.go` to append the label when set.
- Clear `busyLabel` whenever `m.busy` becomes false (at each `m.busy = false` site).
- Set `state.busyLabel = fmt.Sprintf("Creating branch %s...", parsed.branch)` before returning `prepareBranchVibeCmd` in:
  - `dispatchGlobalCommand` (branch vibe)
  - `dispatchAppCommand` (branch vibe)

**Step 4: Run test to verify it passes**

Run:
```bash
mise exec -- go test ./cmd/viberun -run TestRenderLoadingLineShowsBusyLabel
```
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/shell.go cmd/viberun/shell_model.go cmd/viberun/shell_commands.go cmd/viberun/shell_formatting_test.go
git commit -m "client: label spinner during branch creation"
```

---

### Task 2: Auto-attach to base after branch apply

**Files:**
- Modify: `cmd/viberun-server/hostrpc.go`
- Create: `cmd/viberun-server/hostrpc_branch_apply_test.go`
- Modify: `bin/vrctl`
- Modify: `bin/viberun-apply`
- Modify: `config/AGENTS.app.md`

**Step 1: Write the failing test**

Create `cmd/viberun-server/hostrpc_branch_apply_test.go`:

```go
func TestHostRPCBranchApplyAttachFromBranch(t *testing.T) {
	baseDir := t.TempDir()
	origBaseDir := homeVolumeBaseDir
	homeVolumeBaseDir = baseDir
	t.Cleanup(func() { homeVolumeBaseDir = origBaseDir })

	meta := branchMeta{BaseApp: "myapp", Branch: "rainbow"}
	if err := writeBranchMetaAt(homeVolumeBaseDir, "myapp--rainbow", meta); err != nil {
		t.Fatalf("write branch meta: %v", err)
	}

	socketDir, err := os.MkdirTemp("/tmp", "vbr-open-")
	if err != nil {
		t.Fatalf("temp socket dir: %v", err)
	}
	socketPath := filepath.Join(socketDir, "open.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen socket: %v", err)
	}
	defer ln.Close()

	attachApp := ""
	attachSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		attachApp = strings.TrimSpace(r.Form.Get("app"))
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = attachSrv.Serve(ln) }()
	t.Cleanup(func() { _ = attachSrv.Close() })
	t.Setenv("VIBERUN_XDG_OPEN_SOCKET", socketPath)

	gotBase := ""
	gotBranch := ""
	server := &hostRPCServer{
		token: "token",
		app:   "myapp--rainbow",
		branchApplyFn: func(base, branch string) error {
			gotBase = base
			gotBranch = branch
			return nil
		},
	}
	server.httpServer = &http.Server{Handler: server.routes()}

	req := httptest.NewRequest(http.MethodPost, "http://unix/branch/apply?attach=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	server.routes().ServeHTTP(w, req)
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if gotBase != "myapp" || gotBranch != "rainbow" {
		t.Fatalf("unexpected apply args: %q %q", gotBase, gotBranch)
	}
	if attachApp != "myapp" {
		t.Fatalf("expected attach app myapp, got %q", attachApp)
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
mise exec -- go test ./cmd/viberun-server -run TestHostRPCBranchApplyAttachFromBranch
```
Expected: FAIL because `/branch/apply` doesn’t attach or resolve branch metadata from a branch app yet.

**Step 3: Write minimal implementation**

- In `handleBranchApply`:
  - If request body empty and `s.app` is a branch, read `branch.json` for base/branch.
  - Use `branchApplyFn(base, branch)` directly (not `branchApplyFromApp`).
  - After success, if `attach=1` and `base != s.app`, call `sendAttachRequest(base, "")`.
- Add `--attach` support to `vrctl host branch apply`, defaulting to `attach=1` when no branch name.
- Update `bin/viberun-apply` to call `vrctl host branch apply --attach`.
- Update `config/AGENTS.app.md` so “Apply this branch” from a branch auto-switches to base.

**Step 4: Run test to verify it passes**

Run:
```bash
mise exec -- go test ./cmd/viberun-server -run TestHostRPCBranchApplyAttachFromBranch
```
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun-server/hostrpc.go cmd/viberun-server/hostrpc_branch_apply_test.go bin/vrctl bin/viberun-apply config/AGENTS.app.md
git commit -m "server: attach after branch apply"
```

---

### Task 3: Rebase onto local main

**Step 1: Rebase**

Run:
```bash
git rebase main
```
Resolve conflicts if any, then continue:
```bash
git rebase --continue
```

**Step 2: Full test pass**

Run:
```bash
mise exec -- go test ./...
```
Expected: PASS.

**Step 3: Final status check**

Run:
```bash
git status -sb
```
Expected: clean working tree.

---

### Task 4: Deploy (if requested)

**Step 1: Build client/server binaries**

```bash
mise exec -- go build ./cmd/viberun
mise exec -- env GOOS=linux GOARCH=amd64 go build -o /tmp/viberun-server-linux ./cmd/viberun-server
```

**Step 2: Deploy to host**

```bash
scp /tmp/viberun-server-linux root@<host>:/tmp/viberun-server-linux
ssh root@<host> 'install -m 0755 /tmp/viberun-server-linux /usr/local/bin/viberun-server'
```

**Step 3: Rebuild image + rebootstrap (for vrctl + AGENTS)**

Follow existing viberun image build + rebootstrap steps.
