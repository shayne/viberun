// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildShadowCommitsWithMainDefaultBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_TEST_DEFAULT_INITIAL_BRANCH_NAME", "main")
	repoDir := filepath.Join(t.TempDir(), "shadow.git")
	if err := runGitCommand("", "init", "--bare", repoDir); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}
	baseSnapshot := t.TempDir()
	baseApp := t.TempDir()
	branchApp := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseSnapshot, "file.txt"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write base snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseApp, "file.txt"), []byte("main"), 0o644); err != nil {
		t.Fatalf("write base app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(branchApp, "file.txt"), []byte("branch"), 0o644); err != nil {
		t.Fatalf("write branch app: %v", err)
	}
	if err := buildShadowCommits(repoDir, baseSnapshot, baseApp, branchApp, "rainbow"); err != nil {
		t.Fatalf("buildShadowCommits: %v", err)
	}
}
