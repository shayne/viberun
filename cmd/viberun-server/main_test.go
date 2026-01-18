// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestTmuxSessionArgsUsesDefaults(t *testing.T) {
	args := tmuxSessionArgs("", nil)
	if len(args) < 6 {
		t.Fatalf("expected tmux args, got %v", args)
	}
	if args[0] != "tmux" || args[1] != "new-session" {
		t.Fatalf("unexpected tmux prefix: %v", args[:2])
	}
	if args[2] != "-A" || args[3] != "-s" || args[4] != "viberun-session" {
		t.Fatalf("unexpected session args: %v", args[:5])
	}
	if args[5] != "/bin/bash" {
		t.Fatalf("expected default command /bin/bash, got %q", args[5])
	}
}

func TestTmuxSessionArgsKeepsCommand(t *testing.T) {
	command := []string{"codex", "--help"}
	args := tmuxSessionArgs("viberun-agent", command)
	if args[0] != "tmux" || args[1] != "new-session" {
		t.Fatalf("unexpected tmux prefix: %v", args[:2])
	}
	if args[4] != "viberun-agent" {
		t.Fatalf("expected session name, got %q", args[4])
	}
	if args[5] != "codex" || args[6] != "--help" {
		t.Fatalf("unexpected command args: %v", args[5:])
	}
}

func TestDockerExecArgsIncludesEnv(t *testing.T) {
	args := dockerExecArgs("viberun-test", []string{"codex"}, true, map[string]string{
		"COLORTERM": "truecolor",
		"TERM":      "xterm-256color",
	})
	expected := []string{"exec", "-i", "-t", "-e", "COLORTERM=truecolor", "-e", "TERM=xterm-256color", "viberun-test", "codex"}
	if len(args) != len(expected) {
		t.Fatalf("unexpected args length: got %v want %v", args, expected)
	}
	for i, value := range expected {
		if args[i] != value {
			t.Fatalf("unexpected arg at %d: got %q want %q (args=%v)", i, args[i], value, args)
		}
	}
}

func TestDockerExecArgsWithoutTTY(t *testing.T) {
	args := dockerExecArgs("viberun-test", []string{"bash"}, false, map[string]string{
		"TERM": "xterm-256color",
	})
	expected := []string{"exec", "-i", "-e", "TERM=xterm-256color", "viberun-test", "bash"}
	if len(args) != len(expected) {
		t.Fatalf("unexpected args length: got %v want %v", args, expected)
	}
	for i, value := range expected {
		if args[i] != value {
			t.Fatalf("unexpected arg at %d: got %q want %q (args=%v)", i, args[i], value, args)
		}
	}
}

func TestNormalizeTermValue(t *testing.T) {
	cases := map[string]string{
		"":                "xterm-256color",
		"  ":              "xterm-256color",
		"xterm-256color":  "xterm-256color",
		"xterm-ghostty":   "xterm-ghostty",
		"ghostty":         "ghostty",
		"Ghostty":         "Ghostty",
		"screen-256color": "screen-256color",
	}
	for input, expected := range cases {
		if got := normalizeTermValue(input); got != expected {
			t.Fatalf("normalizeTermValue(%q)=%q want %q", input, got, expected)
		}
	}
}
