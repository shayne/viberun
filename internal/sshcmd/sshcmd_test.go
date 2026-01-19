// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshcmd

import "testing"

func TestRemoteArgsDefaultsAgent(t *testing.T) {
	args := RemoteArgs("myapp", "", nil, nil)
	if len(args) < 4 {
		t.Fatalf("expected at least 4 args, got %d", len(args))
	}
	if args[0] != "viberun-server" || args[1] != "--agent" || args[2] != "codex" || args[3] != "myapp" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestRemoteArgsIncludesAgentCheckEnv(t *testing.T) {
	t.Setenv("VIBERUN_AGENT_CHECK", "viberun-agent-check")
	args := RemoteArgs("myapp", "codex", nil, nil)
	if len(args) < 6 {
		t.Fatalf("expected env-prefixed args, got %v", args)
	}
	if args[0] != "env" || args[1] != "VIBERUN_AGENT_CHECK=viberun-agent-check" {
		t.Fatalf("unexpected env prefix: %#v", args[:2])
	}
	if args[2] != "viberun-server" || args[3] != "--agent" || args[4] != "codex" || args[5] != "myapp" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestRemoteArgsIncludesExtraEnv(t *testing.T) {
	t.Setenv("VIBERUN_AGENT_CHECK", "viberun-agent-check")
	args := RemoteArgs("myapp", "codex", []string{"shell"}, map[string]string{
		"VIBERUN_XDG_OPEN_SOCKET": "/tmp/viberun-open.sock",
	})
	if len(args) < 8 {
		t.Fatalf("expected env-prefixed args, got %v", args)
	}
	if args[0] != "env" {
		t.Fatalf("expected env prefix, got %v", args)
	}
	if args[1] != "VIBERUN_AGENT_CHECK=viberun-agent-check" {
		t.Fatalf("unexpected first env: %v", args[1])
	}
	if args[2] != "VIBERUN_XDG_OPEN_SOCKET=/tmp/viberun-open.sock" {
		t.Fatalf("unexpected second env: %v", args[2])
	}
	if args[3] != "viberun-server" || args[4] != "--agent" || args[5] != "codex" || args[6] != "myapp" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestWithSudoKeepsRoot(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp"}
	out := WithSudo("root@host-a", remote)
	if len(out) != len(remote) {
		t.Fatalf("unexpected args: %v", out)
	}
	for i := range remote {
		if out[i] != remote[i] {
			t.Fatalf("unexpected arg at %d: got %q want %q", i, out[i], remote[i])
		}
	}
}

func TestWithSudoWrapsNonRoot(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp"}
	out := WithSudo("alex@host-a", remote)
	expected := []string{"sudo", "-n", "/usr/local/bin/viberun-server", "--agent", "codex", "myapp"}
	if len(out) != len(expected) {
		t.Fatalf("unexpected args: %v", out)
	}
	for i := range expected {
		if out[i] != expected[i] {
			t.Fatalf("unexpected arg at %d: got %q want %q", i, out[i], expected[i])
		}
	}
}

func TestWithSudoPreservesEnvPrefix(t *testing.T) {
	remote := []string{"env", "FOO=bar", "viberun-server", "--agent", "codex", "myapp"}
	out := WithSudo("alex@host-a", remote)
	expected := []string{"env", "FOO=bar", "sudo", "-n", "/usr/local/bin/viberun-server", "--agent", "codex", "myapp"}
	if len(out) != len(expected) {
		t.Fatalf("unexpected args: %v", out)
	}
	for i := range expected {
		if out[i] != expected[i] {
			t.Fatalf("unexpected arg at %d: got %q want %q", i, out[i], expected[i])
		}
	}
}

func TestBuildArgsTTY(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp"}
	args := BuildArgs("host-a", remote, true)
	if len(args) < 2 {
		t.Fatalf("expected args, got %v", args)
	}
	if args[0] != "-tt" {
		t.Fatalf("expected -tt, got %q", args[0])
	}
	if args[1] != "host-a" {
		t.Fatalf("expected host-a, got %q", args[1])
	}
}

func TestBuildArgsNoTTY(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp", "snapshot"}
	args := BuildArgs("host-a", remote, false)
	if len(args) < 2 {
		t.Fatalf("expected args, got %v", args)
	}
	if args[0] != "-T" {
		t.Fatalf("expected -T, got %q", args[0])
	}
	if args[1] != "host-a" {
		t.Fatalf("expected host-a, got %q", args[1])
	}
}

func TestBuildArgsWithLocalForward(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp"}
	forward := &LocalForward{
		LocalPort:  8080,
		RemoteHost: "localhost",
		RemotePort: 8080,
	}
	args := BuildArgsWithLocalForward("host-a", remote, true, forward)
	if len(args) < 4 {
		t.Fatalf("expected args, got %v", args)
	}
	if args[0] != "-tt" {
		t.Fatalf("expected -tt, got %q", args[0])
	}
	if args[1] != "-L" {
		t.Fatalf("expected -L, got %q", args[1])
	}
	if args[2] != "8080:localhost:8080" {
		t.Fatalf("unexpected forward args: %q", args[2])
	}
	if args[3] != "host-a" {
		t.Fatalf("expected host-a, got %q", args[3])
	}
}

func TestBuildArgsWithRemoteSocketForward(t *testing.T) {
	remote := []string{"viberun-server", "--agent", "codex", "myapp"}
	remoteSocket := &RemoteSocketForward{
		RemotePath: "/tmp/viberun-open.sock",
		LocalHost:  "localhost",
		LocalPort:  51234,
	}
	args := BuildArgsWithForwards("host-a", remote, true, nil, remoteSocket)
	if len(args) < 9 {
		t.Fatalf("expected args, got %v", args)
	}
	if args[0] != "-tt" {
		t.Fatalf("expected -tt, got %q", args[0])
	}
	if args[1] != "-o" || args[2] != "ExitOnForwardFailure=yes" {
		t.Fatalf("unexpected forward options: %v", args[1:3])
	}
	if args[3] != "-o" || args[4] != "StreamLocalBindUnlink=yes" {
		t.Fatalf("unexpected forward options: %v", args[3:5])
	}
	if args[5] != "-o" || args[6] != "StreamLocalBindMask=0111" {
		t.Fatalf("unexpected forward options: %v", args[5:7])
	}
	if args[7] != "-R" {
		t.Fatalf("expected -R, got %q", args[7])
	}
	if args[8] != "/tmp/viberun-open.sock:localhost:51234" {
		t.Fatalf("unexpected remote forward: %v", args[8])
	}
}

func TestBuildPortForwardArgs(t *testing.T) {
	forward := &LocalForward{
		LocalPort:  9000,
		RemoteHost: "localhost",
		RemotePort: 9001,
	}
	args := BuildPortForwardArgs("host-a", forward)
	expected := []string{"-T", "-N", "-o", "ExitOnForwardFailure=yes", "-L", "9000:localhost:9001", "host-a"}
	if len(args) != len(expected) {
		t.Fatalf("unexpected args length: got %v want %v", args, expected)
	}
	for i, value := range expected {
		if args[i] != value {
			t.Fatalf("unexpected arg at %d: got %q want %q (args=%v)", i, args[i], value, args)
		}
	}
}
