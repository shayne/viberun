# Custom Dialog Prompts Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all stdin prompts with Crush-style in-TUI dialogs for TTY sessions, while preserving non-TTY fallbacks.

**Architecture:** Add a reusable dialog package (form/confirm/select) built on Bubble Tea v2 + Bubbles v2 textinput. Shell commands will run dialog flows inside the shell model and emit explicit plans (setup/proxy/wipe/etc) that are executed after the shell exits raw mode. CLI commands will reuse the same dialogs via a small runner when a TTY is present and fall back to existing line prompts otherwise.

**Tech Stack:** Go, charm.land/bubbletea/v2, charm.land/bubbles/v2, charm.land/lipgloss/v2, internal/tui/theme.

---

### Task 1: Add dialog foundation + form dialog

**Files:**
- Create: `internal/tui/dialogs/dialog.go`
- Create: `internal/tui/dialogs/form.go`
- Create: `internal/tui/dialogs/form_test.go`

**Step 1: Write the failing test**

```go
// internal/tui/dialogs/form_test.go
func TestFormDialog_SubmitCollectsValues(t *testing.T) {
	fields := []Field{
		{ID: "host", Title: "Server login", Placeholder: "user@host"},
		{ID: "user", Title: "Username", Required: true},
	}
	d := NewFormDialog("setup", "Setup", "Connect", fields)
	d.SetValue("host", "root@1.2.3.4")
	d.SetValue("user", "admin")

	values, ok := d.Values()
	if !ok {
		t.Fatalf("expected dialog to be complete")
	}
	if values["host"] != "root@1.2.3.4" || values["user"] != "admin" {
		t.Fatalf("unexpected values: %#v", values)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/dialogs -run TestFormDialog_SubmitCollectsValues -v`
Expected: FAIL with “undefined: NewFormDialog” (or similar).

**Step 3: Write minimal implementation**

```go
// internal/tui/dialogs/dialog.go
package dialogs

import tea "charm.land/bubbletea/v2"

type Dialog interface {
	ID() string
	Init() tea.Cmd
	Update(tea.Msg) (Dialog, tea.Cmd)
	View() string
	Cursor() *tea.Cursor
}

type Result struct {
	Cancelled bool
	Values    map[string]string
	Choice    string
	Choices   []string
	Confirmed bool
}
```

```go
// internal/tui/dialogs/form.go
package dialogs

import (
	"cmp"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Field struct {
	ID          string
	Title       string
	Description string
	Placeholder string
	Required    bool
	Secret      bool
	Default     string
}

type FormDialog struct {
	id          string
	title       string
	description string
	fields      []Field
	inputs      []textinput.Model
	focused     int
	result      *Result
}

func NewFormDialog(id, title, description string, fields []Field) *FormDialog {
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = cmp.Or(f.Placeholder, f.Description)
		if f.Secret {
			ti.EchoMode = textinput.EchoPassword
		}
		if f.Default != "" {
			ti.SetValue(f.Default)
			ti.CursorEnd()
		}
		ti.SetVirtualCursor(false)
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	return &FormDialog{id: id, title: title, description: description, fields: fields, inputs: inputs}
}

func (d *FormDialog) ID() string { return d.id }
func (d *FormDialog) Init() tea.Cmd { return nil }

func (d *FormDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	// Minimal: forward to focused input; Enter cycles, last Enter submits
	// On submit, validate required fields and set d.result.
	return d, nil
}

func (d *FormDialog) View() string {
	// Render title, description, labels, inputs.
	return lipgloss.JoinVertical(lipgloss.Left, d.title)
}

func (d *FormDialog) Cursor() *tea.Cursor {
	if len(d.inputs) == 0 {
		return nil
	}
	return d.inputs[d.focused].Cursor()
}

func (d *FormDialog) Values() (map[string]string, bool) {
	if d.result == nil || d.result.Cancelled {
		return nil, false
	}
	return d.result.Values, true
}

func (d *FormDialog) SetValue(id, value string) {
	for i, f := range d.fields {
		if f.ID == id {
			d.inputs[i].SetValue(value)
			d.inputs[i].CursorEnd()
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/dialogs -run TestFormDialog_SubmitCollectsValues -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/dialogs/dialog.go internal/tui/dialogs/form.go internal/tui/dialogs/form_test.go

git commit -m "client: add form dialog component"
```

---

### Task 2: Add confirm dialog (yes/no) with defaults

**Files:**
- Create: `internal/tui/dialogs/confirm.go`
- Create: `internal/tui/dialogs/confirm_test.go`

**Step 1: Write the failing test**

```go
func TestConfirmDialog_DefaultYes(t *testing.T) {
	d := NewConfirmDialog("wipe", "Wipe server?", "This removes all data.", true)
	d.SubmitDefault()
	result, ok := d.Result()
	if !ok || !result.Confirmed {
		t.Fatalf("expected default yes confirmation")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/dialogs -run TestConfirmDialog_DefaultYes -v`
Expected: FAIL (undefined NewConfirmDialog).

**Step 3: Write minimal implementation**

```go
// internal/tui/dialogs/confirm.go
package dialogs

import tea "charm.land/bubbletea/v2"

type ConfirmDialog struct {
	id          string
	title       string
	description string
	defaultYes  bool
	result      *Result
}

func NewConfirmDialog(id, title, description string, defaultYes bool) *ConfirmDialog {
	return &ConfirmDialog{id: id, title: title, description: description, defaultYes: defaultYes}
}

func (d *ConfirmDialog) ID() string { return d.id }
func (d *ConfirmDialog) Init() tea.Cmd { return nil }
func (d *ConfirmDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	// Handle y/n/enter/esc -> set d.result
	return d, nil
}
func (d *ConfirmDialog) View() string { return d.title }
func (d *ConfirmDialog) Cursor() *tea.Cursor { return nil }
func (d *ConfirmDialog) Result() (*Result, bool) {
	if d.result == nil { return nil, false }
	return d.result, true
}
func (d *ConfirmDialog) SubmitDefault() { d.result = &Result{Confirmed: d.defaultYes} }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/dialogs -run TestConfirmDialog_DefaultYes -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/dialogs/confirm.go internal/tui/dialogs/confirm_test.go

git commit -m "client: add confirm dialog"
```

---

### Task 3: Add select and multi-select dialogs

**Files:**
- Create: `internal/tui/dialogs/select.go`
- Create: `internal/tui/dialogs/select_test.go`

**Step 1: Write the failing test**

```go
func TestSelectDialog_DefaultChoice(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	d := NewSelectDialog("agent", "Choose", "", options, "b")
	choice, ok := d.Result()
	if !ok || choice.Choice != "b" {
		t.Fatalf("expected default choice b")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/dialogs -run TestSelectDialog_DefaultChoice -v`
Expected: FAIL (undefined NewSelectDialog).

**Step 3: Write minimal implementation**

```go
// internal/tui/dialogs/select.go
package dialogs

import tea "charm.land/bubbletea/v2"

type Option struct { Label, Value string }

type SelectDialog struct {
	id          string
	title       string
	description string
	options     []Option
	index       int
	result      *Result
}

func NewSelectDialog(id, title, description string, options []Option, defaultValue string) *SelectDialog {
	idx := 0
	for i, opt := range options {
		if opt.Value == defaultValue { idx = i; break }
	}
	return &SelectDialog{id: id, title: title, description: description, options: options, index: idx}
}

func (d *SelectDialog) ID() string { return d.id }
func (d *SelectDialog) Init() tea.Cmd { return nil }
func (d *SelectDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	// up/down to move, enter to select
	return d, nil
}
func (d *SelectDialog) View() string { return d.title }
func (d *SelectDialog) Cursor() *tea.Cursor { return nil }
func (d *SelectDialog) Result() (*Result, bool) {
	if d.result == nil { return nil, false }
	return d.result, true
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/dialogs -run TestSelectDialog_DefaultChoice -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/dialogs/select.go internal/tui/dialogs/select_test.go

git commit -m "client: add select dialogs"
```

---

### Task 4: Add standalone dialog runner + upgrade existing prompt helpers (CLI path)

**Files:**
- Create: `internal/tui/dialogs/runner.go`
- Modify: `internal/tui/prompt.go`
- Modify: `internal/tui/password_prompt.go`
- Modify: `internal/tui/setup_prompt.go`
- Modify: `internal/tui/proxy_prompt.go`
- Modify: `internal/tui/proxy_ip_prompt.go`
- Modify: `internal/tui/proxy_auth_prompt.go`
- Modify: `internal/tui/wipe_prompt.go`
- Modify: `internal/tui/agent_select.go`
- Modify: `cmd/viberun/main.go`

**Step 1: Write the failing test**

```go
func TestPromptPassword_NonTTYFallback(t *testing.T) {
	in := strings.NewReader("secret\n")
	var out bytes.Buffer
	got, err := PromptPassword(in, &out, "Password")
	if err != nil || got != "secret" {
		t.Fatalf("unexpected result: %q err=%v", got, err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestPromptPassword_NonTTYFallback -v`
Expected: FAIL (compile error until runner exists).

**Step 3: Write minimal implementation**

```go
// internal/tui/dialogs/runner.go
package dialogs

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

type runnerModel struct {
	dialog Dialog
	result *Result
}

func (m runnerModel) Init() tea.Cmd { return m.dialog.Init() }
func (m runnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	d, cmd := m.dialog.Update(msg)
	m.dialog = d
	if res, ok := resultFromDialog(d); ok {
		m.result = res
		return m, tea.Quit
	}
	return m, cmd
}
func (m runnerModel) View() tea.View { return tea.NewView(m.dialog.View()) }
func (m runnerModel) Cursor() *tea.Cursor { return m.dialog.Cursor() }

func Run(in io.Reader, out io.Writer, d Dialog) (*Result, error) {
	p := tea.NewProgram(runnerModel{dialog: d}, tea.WithInput(in), tea.WithOutput(out))
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	if rm, ok := m.(runnerModel); ok {
		return rm.result, nil
	}
	return nil, nil
}
```

Update prompt helpers to use `dialogs.Run` when both input/output are TTYs; otherwise keep current line-based behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestPromptPassword_NonTTYFallback -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/dialogs/runner.go internal/tui/*.go cmd/viberun/main.go

git commit -m "client: route CLI prompts through dialogs"
```

---

### Task 5: Add shell dialog host + shared prompt flow engine

**Files:**
- Create: `cmd/viberun/prompt_flow.go`
- Modify: `cmd/viberun/shell.go`
- Modify: `cmd/viberun/shell_model.go`
- Modify: `cmd/viberun/shell_commands.go`
- Create: `cmd/viberun/prompt_flow_test.go`

**Step 1: Write the failing test**

```go
func TestPromptFlow_ConfirmCancel(t *testing.T) {
	flow := newConfirmFlow("delete", "Delete app?", "", false)
	if flow.Done() {
		t.Fatalf("expected flow to start incomplete")
	}
	flow.ApplyResult(dialogs.Result{Cancelled: true})
	if !flow.Cancelled() {
		t.Fatalf("expected flow to cancel")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/viberun -run TestPromptFlow_ConfirmCancel -v`
Expected: FAIL (undefined flow types).

**Step 3: Write minimal implementation**

```go
// cmd/viberun/prompt_flow.go
package main

import "github.com/shayne/viberun/internal/tui/dialogs"

type promptFlow interface {
	Dialog() dialogs.Dialog
	ApplyResult(dialogs.Result)
	Done() bool
	Cancelled() bool
}
```

Wire the shell model to:
- Store `activeFlow promptFlow` in `shellState`.
- Route key messages to `activeFlow.Dialog().Update` when active.
- On dialog submit/cancel, call `activeFlow.ApplyResult`, advance or close.
- When flow completes, set a pending action (e.g. `state.shellAction`) and `tea.Quit` to run it outside raw mode.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/viberun -run TestPromptFlow_ConfirmCancel -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/prompt_flow.go cmd/viberun/shell.go cmd/viberun/shell_model.go cmd/viberun/shell_commands.go cmd/viberun/prompt_flow_test.go

git commit -m "client: add shell prompt flow host"
```

---

### Task 6: Implement setup/proxy/wipe/password flows and remove remaining stdin prompts

**Files:**
- Modify: `cmd/viberun/shell_setup.go`
- Modify: `cmd/viberun/shell_actions.go`
- Modify: `cmd/viberun/wipe_flow.go`
- Modify: `cmd/viberun/main.go`
- Modify: `cmd/viberun/shell_commands.go`
- Modify: `internal/tui/setup_prompt.go`
- Modify: `internal/tui/proxy_prompt.go`
- Modify: `internal/tui/proxy_ip_prompt.go`
- Modify: `internal/tui/proxy_auth_prompt.go`
- Modify: `internal/tui/wipe_prompt.go`
- Modify: `internal/tui/password_prompt.go`
- Create: `cmd/viberun/setup_flow_test.go`

**Step 1: Write the failing test**

```go
func TestSetupFlow_PrefillsHost(t *testing.T) {
	flow := newSetupFlow(setupFlowInput{ExistingHost: "root@1.2.3.4"})
	d := flow.Dialog().(*dialogs.FormDialog)
	values, _ := d.Values()
	if values["host"] != "root@1.2.3.4" {
		t.Fatalf("expected host to be prefilled")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/viberun -run TestSetupFlow_PrefillsHost -v`
Expected: FAIL (undefined setup flow).

**Step 3: Write minimal implementation**

- Add `setupPlan`, `proxyPlan`, `wipePlan`, `passwordPlan` structs in `cmd/viberun/prompt_flow.go`.
- Implement flows that:
  - **Setup:** rerun confirm (if connected) → host form (prefill existing host, placeholder `user@host  # username + address`) → update artifacts confirm (default yes/no per existing logic) → emit `setupPlan`.
  - **Proxy setup:** read config summary, ask edit/continue, then domain/IP/user/pass forms with defaults, emit `proxyPlan`.
  - **Wipe:** confirm yes/no then `WIPE` token entry, emit `wipePlan`.
  - **Password:** single secret field, emit `passwordPlan`.
- Refactor `runShellSetup`, `runProxySetupFlow`, `runWipeFlow`, and `runShellUsersAdd/SetPassword` to accept plans and skip prompts.
- Replace `promptYesNoDefault*`, `promptDelete`, `promptCreateLocal`, `promptProxySetup`, `promptRecreateApps` with dialog flows when TTY; keep existing non-TTY behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/viberun -run TestSetupFlow_PrefillsHost -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/viberun/*.go internal/tui/*.go

git commit -m "client: move setup/proxy/wipe prompts into dialogs"
```

---

### Task 7: Update tests and run full suite

**Files:**
- Modify: `cmd/viberun/legacy_snapshot_test.go` (if output changes)
- Modify: `internal/ui/testdata/*.txt` (if prompt text changes)

**Step 1: Run targeted tests**

Run: `go test ./cmd/viberun -run Prompt -v`
Expected: PASS.

**Step 2: Run full suite**

Run: `go test ./...`
Expected: PASS.

**Step 3: Commit**

```bash
git add cmd/viberun/*.go internal/ui/testdata/*.txt

git commit -m "client: update prompt tests"
```

---

### Notes / Invariants to Preserve
- Keep prompt text and default behaviors identical to current UX (only rendering changes).
- Keep non-TTY behavior unchanged (stdin prompts still work in pipelines).
- Maintain the two-step wipe confirmation and existing proxy setup branching.
- Use the Charm v2 stack only (no v1 Huh usage).
