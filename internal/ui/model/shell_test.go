// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestShellCtrlLClearsOutput(t *testing.T) {
	m := NewShellModel()
	m.output = []string{"line"}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	um := updated.(ShellModel)
	if len(um.output) != 0 {
		t.Fatalf("expected output cleared")
	}
}

func TestShellEnterEmptyAddsPromptLine(t *testing.T) {
	m := NewShellModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(ShellModel)
	if len(um.output) != 1 {
		t.Fatalf("expected 1 output line, got %d", len(um.output))
	}
	if um.output[0] != "viberun > " {
		t.Fatalf("unexpected prompt line: %q", um.output[0])
	}
}

func TestShellViewShowsPromptWhenOutputEmpty(t *testing.T) {
	m := NewShellModel()
	view := m.View()
	got := fmt.Sprint(view.Content)
	if got != "viberun > " {
		t.Fatalf("unexpected view content: %q", got)
	}
}

func TestShellViewAddsBlankLineAndPromptAfterOutput(t *testing.T) {
	m := NewShellModel()
	m.output = []string{"hello"}
	view := m.View()
	got := fmt.Sprint(view.Content)
	want := "hello\n\nviberun > "
	if got != want {
		t.Fatalf("unexpected view content: %q", got)
	}
}

func TestAppendCommandLineInsertsBlankLineAfterOutput(t *testing.T) {
	m := NewShellModel()
	m.output = []string{"line"}
	m.appendCommandLine("help")
	if len(m.output) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(m.output))
	}
	if m.output[1] != "" {
		t.Fatalf("expected blank separator, got %q", m.output[1])
	}
	if m.output[2] != "viberun > help" {
		t.Fatalf("unexpected prompt line: %q", m.output[2])
	}
}

func TestPromptPrefixUsesAppScope(t *testing.T) {
	m := NewShellModel()
	m.scope = scopeAppConfig
	m.app = "myapp"
	if got := m.promptPrefix(); got != "viberun myapp > " {
		t.Fatalf("unexpected prompt prefix: %q", got)
	}
}

func TestShellInputAppendsText(t *testing.T) {
	m := NewShellModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	um := updated.(ShellModel)
	if um.input != "a" {
		t.Fatalf("expected input to be %q, got %q", "a", um.input)
	}
}

func TestShellInputBackspace(t *testing.T) {
	m := NewShellModel()
	m.input = "ab"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	um := updated.(ShellModel)
	if um.input != "a" {
		t.Fatalf("expected input to be %q, got %q", "a", um.input)
	}
}

func TestShellViewIncludesInput(t *testing.T) {
	m := NewShellModel()
	m.input = "foo"
	view := m.View()
	got := fmt.Sprint(view.Content)
	want := "viberun > foo"
	if got != want {
		t.Fatalf("unexpected view content: %q", got)
	}
}

type testBackend struct {
	called   int
	lastLine string
	output   string
}

func (b *testBackend) Dispatch(line string) (string, tea.Cmd) {
	b.called++
	b.lastLine = line
	return b.output, nil
}

func TestShellDispatchesToBackend(t *testing.T) {
	backend := &testBackend{output: "backend output"}
	m := NewShellModel()
	m.backend = backend
	m.input = "status"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(ShellModel)
	if backend.called != 1 {
		t.Fatalf("expected backend called once, got %d", backend.called)
	}
	if backend.lastLine != "status" {
		t.Fatalf("unexpected backend line: %q", backend.lastLine)
	}
	if len(um.output) == 0 || um.output[len(um.output)-1] != "backend output" {
		t.Fatalf("missing backend output, got %v", um.output)
	}
}
