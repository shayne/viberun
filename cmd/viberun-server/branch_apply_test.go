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

func TestEnsureShadowRepoExisting(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "shadow.git")
	if err := runGitCommand("", "init", "--bare", repo); err != nil {
		t.Fatalf("init bare repo: %v", err)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("expected repo dir: %v", err)
	}
	if err := ensureShadowRepo(repo); err != nil {
		t.Fatalf("ensureShadowRepo: %v", err)
	}
}
