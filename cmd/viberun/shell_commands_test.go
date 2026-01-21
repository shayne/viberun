// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestSplitShellArgs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{input: "apps", want: []string{"apps"}},
		{input: "app myapp", want: []string{"app", "myapp"}},
		{input: "config set host root@1.2.3.4", want: []string{"config", "set", "host", "root@1.2.3.4"}},
		{input: "config set agent npx:@sourcegraph/amp@latest", want: []string{"config", "set", "agent", "npx:@sourcegraph/amp@latest"}},
		{input: "url set-domain myapp.com", want: []string{"url", "set-domain", "myapp.com"}},
		{input: "app my\\ app", want: []string{"app", "my app"}},
		{input: "config set host 'root@my-host'", want: []string{"config", "set", "host", "root@my-host"}},
		{input: "config set host \"root@my host\"", want: []string{"config", "set", "host", "root@my host"}},
	}
	for _, tc := range cases {
		got, err := splitShellArgs(tc.input)
		if err != nil {
			t.Fatalf("splitShellArgs(%q) returned error: %v", tc.input, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("splitShellArgs(%q)=%v want %v", tc.input, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitShellArgs(%q)=%v want %v", tc.input, got, tc.want)
			}
		}
	}
}

func TestSplitShellArgsUnterminated(t *testing.T) {
	_, err := splitShellArgs("config set host 'root@host")
	if err == nil {
		t.Fatalf("expected error for unterminated quote")
	}
}

func TestParseShellCommandLowercase(t *testing.T) {
	cmd, err := parseShellCommand("RuN MyApp")
	if err != nil {
		t.Fatalf("parseShellCommand returned error: %v", err)
	}
	if cmd.name != "run" {
		t.Fatalf("expected name 'run', got %q", cmd.name)
	}
	if len(cmd.args) != 1 || cmd.args[0] != "MyApp" {
		t.Fatalf("unexpected args: %v", cmd.args)
	}
	if cmd.enforceExisting {
		t.Fatalf("expected enforceExisting=false for parsed run command")
	}
}

func TestDispatchDefersRemoteCommandUntilSync(t *testing.T) {
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: false,
	}
	result, cmd := dispatchShellCommand(state, "apps")
	if result != "" {
		t.Fatalf("expected no immediate output, got %q", result)
	}
	if cmd == nil {
		t.Fatalf("expected sync command for deferred remote command")
	}
	if state.pendingCmd == nil {
		t.Fatalf("expected pending command to be queued")
	}
	if state.pendingCmd.cmd.name != "apps" {
		t.Fatalf("expected pending apps command, got %q", state.pendingCmd.cmd.name)
	}
}

func TestDispatchRejectsUnknownApp(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: true,
		apps:       []string{"known"},
	}
	result, cmd := dispatchShellCommand(state, "app missing")
	if cmd != nil {
		t.Fatalf("expected no command for invalid app")
	}
	if result != "error: app \"missing\" not found" {
		t.Fatalf("unexpected error output: %q", result)
	}
}

func TestDispatchPendingCDUsesAppValidation(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: true,
		apps:       []string{"myapp"},
	}
	pending := &pendingCommand{cmd: parsedCommand{name: "cd", args: []string{"myapp"}}, scope: scopeGlobal}
	result, cmd := dispatchCommandWithScope(state, pending)
	if cmd != nil {
		t.Fatalf("expected no command for pending cd")
	}
	if result != "" {
		t.Fatalf("expected no output, got %q", result)
	}
	if state.scope != scopeAppConfig || state.app != "myapp" {
		t.Fatalf("expected app context set, got scope=%v app=%q", state.scope, state.app)
	}
}

func TestDispatchDefersWhileSyncing(t *testing.T) {
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: true,
		syncing:    true,
	}
	result, cmd := dispatchShellCommand(state, "apps")
	if result != "" {
		t.Fatalf("expected no immediate output, got %q", result)
	}
	if state.pendingCmd == nil || state.pendingCmd.cmd.name != "apps" {
		t.Fatalf("expected pending apps command while syncing")
	}
	if cmd != nil {
		t.Fatalf("expected no new sync command when already syncing")
	}
}

func TestDispatchDefersLsWhileSyncing(t *testing.T) {
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: true,
		syncing:    true,
	}
	result, cmd := dispatchShellCommand(state, "ls")
	if result != "" {
		t.Fatalf("expected no immediate output, got %q", result)
	}
	if state.pendingCmd == nil || state.pendingCmd.cmd.name != "apps" {
		t.Fatalf("expected pending apps command while syncing")
	}
	if cmd != nil {
		t.Fatalf("expected no new sync command when already syncing")
	}
}

func TestRunDotSlashRequiresExisting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	state := &shellState{
		scope:      scopeGlobal,
		appsLoaded: true,
		apps:       []string{"known"},
	}
	result, cmd := dispatchShellCommand(state, "./missing")
	if cmd != nil {
		t.Fatalf("expected no command for missing app")
	}
	if result == "" || !strings.Contains(result, "Run `run missing` to create it") {
		t.Fatalf("unexpected error output: %q", result)
	}
}
