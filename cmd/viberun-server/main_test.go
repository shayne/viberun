// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestTmuxSessionArgsUsesDefaults(t *testing.T) {
	args := tmuxSessionArgs("", "", nil)
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
	args := tmuxSessionArgs("viberun-agent", "codex", command)
	if args[0] != "tmux" || args[1] != "new-session" {
		t.Fatalf("unexpected tmux prefix: %v", args[:2])
	}
	if args[4] != "viberun-agent" {
		t.Fatalf("expected session name, got %q", args[4])
	}
	if args[5] != "-n" || args[6] != "codex" {
		t.Fatalf("expected window name args, got %v", args[5:7])
	}
	if args[7] != "codex" || args[8] != "--help" {
		t.Fatalf("unexpected command args: %v", args[7:])
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

func TestParsePortMapping(t *testing.T) {
	output := "0.0.0.0:49160\n"
	if port, ok := parsePortMapping(output); !ok || port != 49160 {
		t.Fatalf("expected port 49160, got %d ok=%v", port, ok)
	}
	output = ":::49161\n"
	if port, ok := parsePortMapping(output); !ok || port != 49161 {
		t.Fatalf("expected port 49161, got %d ok=%v", port, ok)
	}
	output = "invalid\n"
	if port, ok := parsePortMapping(output); ok || port != 0 {
		t.Fatalf("expected no port, got %d ok=%v", port, ok)
	}
}

func TestResolveSnapshotRef(t *testing.T) {
	ref, err := resolveSnapshotRef("myapp", "customtag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "viberun-snapshot-myapp:customtag" {
		t.Fatalf("unexpected ref: %q", ref)
	}
	ref, err = resolveSnapshotRef("myapp", "repo:tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "repo:tag" {
		t.Fatalf("unexpected ref: %q", ref)
	}
	if _, err := resolveSnapshotRef("myapp", ""); err == nil {
		t.Fatalf("expected error for empty snapshot name")
	}
}
