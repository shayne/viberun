// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os/exec"
	"testing"
)

func TestRunGitCommandSafeBypassesDubiousOwnership(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	if err := runGitCommand(repo, "init"); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	if err := runGitCommand(repo, "status", "-sb"); err == nil {
		t.Fatalf("expected dubious ownership error")
	}
	if err := runGitCommandSafe(repo, repo, "status", "-sb"); err != nil {
		t.Fatalf("safe git command failed: %v", err)
	}
}
