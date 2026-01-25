# Clickable tmux bar buttons Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add mouse-clickable agent/shell/detach controls to the container tmux status bar so users can create/select a `shell` window and detach without keyboard shortcuts.

**Architecture:** Render user-defined status ranges for the agent and `shell` labels in `bin/viberun-tmux-status`, add a click-router script invoked by a `MouseDown1Status` binding, and enable tmux mouse mode in `config/tmux.conf`. Ship the new script into the container via `Dockerfile`.

**Tech Stack:** tmux config, POSIX shell scripts, Go tests.

### Task 1: Status bar ranges for agent + shell

**Files:**
- Create: `internal/tmuxbar/status_test.go`
- Modify: `bin/viberun-tmux-status`

**Step 1: Write the failing test** (uses a stub `tmux` to control window names)

```go
package tmuxbar

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func writeStub(t *testing.T, dir, name, contents string) string {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runStatus(t *testing.T, env map[string]string) string {
	root := repoRoot(t)
	cmd := exec.Command(filepath.Join(root, "bin", "viberun-tmux-status"), "left")
	cmd.Env = append(os.Environ(),
		"VIBERUN_APP=myapp",
		"VIBERUN_AGENT=codex",
	)
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	return out.String()
}

func TestStatusLeft_ShellButtonWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	writeStub(t, tmp, "tmux", "#!/bin/sh\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nexit 0\n")
	output := runStatus(t, map[string]string{
		"PATH": tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_TMUX_WINDOWS": "codex\n",
	})
	if !strings.Contains(output, "viberun myapp") {
		t.Fatalf("expected app label, got %q", output)
	}
	if !strings.Contains(output, "#[range=user|codex]codex#[range=default]") {
		t.Fatalf("expected agent range, got %q", output)
	}
	if !strings.Contains(output, "#[fg=colour240]#[range=user|shell]shell#[range=default]") {
		t.Fatalf("expected shell button style, got %q", output)
	}
}

func TestStatusLeft_ShellTabWhenPresent(t *testing.T) {
	tmp := t.TempDir()
	writeStub(t, tmp, "tmux", "#!/bin/sh\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nexit 0\n")
	output := runStatus(t, map[string]string{
		"PATH": tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_TMUX_WINDOWS": "codex\nshell\n",
	})
	if !strings.Contains(output, "#[fg=colour245]#[range=user|shell]shell#[range=default]") {
		t.Fatalf("expected shell tab style, got %q", output)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `mise exec -- go test ./internal/tmuxbar -run TestStatusLeft_ -v`
Expected: FAIL because `bin/viberun-tmux-status` does not yet render ranges/styles.

**Step 3: Write minimal implementation** (add range markup + shell detection)

```sh
# in bin/viberun-tmux-status
shell_window_exists() {
  tmux list-windows -F '#{window_name}' 2>/dev/null | grep -qx "shell"
}

range_wrap() {
  range="$1"
  text="$2"
  color="$3"
  printf "%s#[range=user|%s]%s#[range=default]%s" "$color" "$range" "$text" "$color_reset"
}

left)
  label="viberun"
  if [ -n "$app" ]; then
    label="$label $app"
  fi
  out="$label"
  if [ -n "$agent" ]; then
    out="$out $(range_wrap "$agent" "$agent" "$color_dim")"
  fi
  if shell_window_exists; then
    out="$out $(range_wrap "shell" "shell" "$color_dim")"
  else
    out="$out $(range_wrap "shell" "shell" "$color_muted")"
  fi
  printf "%s" "$out"
  ;;
```

**Step 4: Run test to verify it passes**

Run: `mise exec -- go test ./internal/tmuxbar -run TestStatusLeft_ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmuxbar/status_test.go bin/viberun-tmux-status
git commit -m "tmux: add status ranges for agent and shell"
```

### Task 2: Click handler script for shell/agent/detach

**Files:**
- Modify: `internal/tmuxbar/status_test.go`
- Create: `bin/viberun-tmux-click`

**Step 1: Write the failing test** (extends the same test file)

```go
func TestClickScript_ShellCreatesWhenMissing(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	log := filepath.Join(tmp, "tmux.log")
	writeStub(t, tmp, "tmux", "#!/bin/sh\nlog=\"${VIBERUN_TMUX_LOG:-}\"\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nif [ -n \"$log\" ]; then\n  printf '%s\\n' \"$*\" >> \"$log\"\nfi\nexit 0\n")
	cmd := exec.Command(filepath.Join(root, "bin", "viberun-tmux-click"), "shell")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"VIBERUN_TMUX_LOG="+log,
		"VIBERUN_TMUX_WINDOWS=codex\n",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "new-window -n shell") {
		t.Fatalf("expected new-window, got %q", string(data))
	}
}

func TestClickScript_Detach(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	log := filepath.Join(tmp, "tmux.log")
	writeStub(t, tmp, "tmux", "#!/bin/sh\nlog=\"${VIBERUN_TMUX_LOG:-}\"\nif [ -n \"$log\" ]; then\n  printf '%s\\n' \"$*\" >> \"$log\"\nfi\nexit 0\n")
	cmd := exec.Command(filepath.Join(root, "bin", "viberun-tmux-click"), "detach")
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"VIBERUN_TMUX_LOG="+log,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "detach-client") {
		t.Fatalf("expected detach-client, got %q", string(data))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `mise exec -- go test ./internal/tmuxbar -run TestClickScript_ -v`
Expected: FAIL because `bin/viberun-tmux-click` does not exist yet.

**Step 3: Write minimal implementation**

```sh
#!/bin/sh
set -eu

range="${1:-}"

case "$range" in
  ""|"window"|"pane"|"session")
    exec tmux select-window -t {mouse}
    ;;
  "shell")
    if tmux list-windows -F '#{window_name}' 2>/dev/null | grep -qx "shell"; then
      exec tmux select-window -t shell
    fi
    exec tmux new-window -n shell
    ;;
  "detach")
    exec tmux detach-client
    ;;
  *)
    exec tmux select-window -t "$range"
    ;;
esac
```

**Step 4: Run test to verify it passes**

Run: `mise exec -- go test ./internal/tmuxbar -run TestClickScript_ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmuxbar/status_test.go bin/viberun-tmux-click
git commit -m "tmux: add clickable status bar handler"
```

### Task 3: tmux config wiring + container install

**Files:**
- Modify: `internal/tmuxbar/status_test.go`
- Modify: `config/tmux.conf`
- Modify: `Dockerfile`

**Step 1: Write the failing test**

```go
func TestTmuxConfHasMouseAndClickBinding(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "config", "tmux.conf"))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if !strings.Contains(contents, "set -g mouse on") {
		t.Fatalf("missing mouse option")
	}
	if !strings.Contains(contents, "MouseDown1Status") || !strings.Contains(contents, "viberun-tmux-click") {
		t.Fatalf("missing MouseDown1Status binding to viberun-tmux-click")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `mise exec -- go test ./internal/tmuxbar -run TestTmuxConfHasMouseAndClickBinding -v`
Expected: FAIL

**Step 3: Write minimal implementation**

```tmux
# in config/tmux.conf
set -g mouse on
bind-key -n MouseDown1Status run-shell "/usr/local/bin/viberun-tmux-click '#{mouse_status_range}'"
```

```Dockerfile
# in Dockerfile
COPY bin/viberun-tmux-click /usr/local/bin/viberun-tmux-click
...
RUN chmod +x /usr/local/bin/viberun-tmux-status \
  && chmod +x /usr/local/bin/viberun-tmux-click \
  && ...
```

**Step 4: Run test to verify it passes**

Run: `mise exec -- go test ./internal/tmuxbar -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmuxbar/status_test.go config/tmux.conf Dockerfile
git commit -m "tmux: wire mouse click bindings into container"
```

## Notes
- Use @superpowers:test-driven-development for each task before implementation.
- Use @superpowers:verification-before-completion before declaring work complete.
