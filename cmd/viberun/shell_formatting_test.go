// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
)

func newTestState(t *testing.T) *shellState {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	return &shellState{
		output: []string{},
		scope:  scopeGlobal,
		host:   "root@host",
		agent:  "codex",
	}
}

func newTestModel(state *shellState) shellModel {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = ""
	input.CharLimit = 0
	input.SetWidth(80)
	input.Blur()
	return shellModel{
		state:  state,
		input:  input,
		width:  80,
		height: 40,
	}
}

func TestShellStateAppendOutput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: []string{""}},
		{name: "single-line", input: "hello", want: []string{"hello"}},
		{name: "trailing-newline", input: "hello\n", want: []string{"hello", ""}},
		{name: "multi-line-trailing-newlines", input: "a\nb\n\n", want: []string{"a", "b", "", ""}},
		{name: "only-newlines", input: "\n\n", want: []string{"", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := newTestState(t)
			state.appendOutput(tc.input)
			if !reflect.DeepEqual(state.output, tc.want) {
				t.Fatalf("appendOutput(%q) = %#v, want %#v", tc.input, state.output, tc.want)
			}
		})
	}
}

func TestAppendCommandLinePreservesBlankLine(t *testing.T) {
	state := newTestState(t)
	state.output = []string{"myapp3"}
	model := newTestModel(state)
	model.appendCommandLine("apps")
	prefix := renderPromptPrefix(state)
	want := []string{"myapp3", "", prefix + "apps"}
	if !reflect.DeepEqual(state.output, want) {
		t.Fatalf("output = %#v, want %#v", state.output, want)
	}
}

func TestAppendCommandLineDoesNotDoubleBlankLine(t *testing.T) {
	state := newTestState(t)
	state.output = []string{"myapp3", ""}
	model := newTestModel(state)
	model.appendCommandLine("apps")
	prefix := renderPromptPrefix(state)
	want := []string{"myapp3", "", prefix + "apps"}
	if !reflect.DeepEqual(state.output, want) {
		t.Fatalf("output = %#v, want %#v", state.output, want)
	}
}

func TestAppendCommandLineAfterPromptLine(t *testing.T) {
	state := newTestState(t)
	prefix := renderPromptPrefix(state)
	state.output = []string{prefix}
	model := newTestModel(state)
	model.appendCommandLine("apps")
	want := []string{prefix, prefix + "apps"}
	if !reflect.DeepEqual(state.output, want) {
		t.Fatalf("output = %#v, want %#v", state.output, want)
	}
}

func TestRenderPromptPrefixAppScope(t *testing.T) {
	state := newTestState(t)
	state.scope = scopeAppConfig
	state.app = "myapp"
	if got, want := renderPromptPrefix(state), "viberun myapp > "; got != want {
		t.Fatalf("renderPromptPrefix() = %q, want %q", got, want)
	}
}

func TestViewNoOutputShowsBlankLineBeforePrompt(t *testing.T) {
	state := newTestState(t)
	model := newTestModel(state)
	view := model.View()
	rendered := fmt.Sprint(view.Content)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d: %q", len(lines), rendered)
	}
	if lines[0] != "" {
		t.Fatalf("expected blank line above header, got %q", lines[0])
	}
	if lines[2] != "" {
		t.Fatalf("expected blank line between header and prompt, got %q", lines[2])
	}
	prefix := renderPromptPrefix(state)
	if !strings.HasPrefix(lines[len(lines)-1], prefix) {
		t.Fatalf("prompt line %q does not start with %q", lines[len(lines)-1], prefix)
	}
}

func TestViewSeparatorAfterClearSpacing(t *testing.T) {
	state := newTestState(t)
	prefix := renderPromptPrefix(state)
	state.output = []string{prefix + "apps", "myapp3"}
	model := newTestModel(state)
	view := model.View()
	rendered := fmt.Sprint(view.Content)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d: %q", len(lines), rendered)
	}
	if lines[0] != "" {
		t.Fatalf("expected blank line above header, got %q", lines[0])
	}
	if lines[2] != "" {
		t.Fatalf("expected blank line after header, got %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], prefix) {
		t.Fatalf("expected first body line to start with %q, got %q", prefix, lines[3])
	}
}

func TestViewInsertsBlankLineBeforePrompt(t *testing.T) {
	state := newTestState(t)
	state.output = []string{"viberun > apps", "myapp3"}
	model := newTestModel(state)
	view := model.View()
	rendered := fmt.Sprint(view.Content)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d: %q", len(lines), rendered)
	}
	if lines[len(lines)-2] != "" {
		t.Fatalf("expected blank line before prompt, got %q", lines[len(lines)-2])
	}
}

func TestViewPreservesTrailingBlankLine(t *testing.T) {
	state := newTestState(t)
	state.output = []string{"viberun > apps", "myapp3", ""}
	model := newTestModel(state)
	view := model.View()
	rendered := fmt.Sprint(view.Content)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 lines, got %d: %q", len(lines), rendered)
	}
	if lines[len(lines)-2] != "" {
		t.Fatalf("expected preserved blank line before prompt, got %q", lines[len(lines)-2])
	}
}

func TestViewNoExtraBlankLineAfterPromptOutput(t *testing.T) {
	state := newTestState(t)
	prefix := renderPromptPrefix(state)
	state.output = []string{prefix}
	model := newTestModel(state)
	view := model.View()
	rendered := fmt.Sprint(view.Content)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d: %q", len(lines), rendered)
	}
	if lines[len(lines)-2] == "" {
		t.Fatalf("expected no blank line between prompt output and prompt, got %q", rendered)
	}
}
