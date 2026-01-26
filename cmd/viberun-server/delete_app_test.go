// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"testing"

	branchpkg "github.com/shayne/viberun/internal/branch"
	"github.com/shayne/viberun/internal/server"
)

func TestDeleteAppRemovesBranches(t *testing.T) {
	baseDir := t.TempDir()
	origBaseDir := homeVolumeBaseDir
	homeVolumeBaseDir = baseDir
	t.Cleanup(func() { homeVolumeBaseDir = origBaseDir })

	base := "myapp"
	branch := "feature"
	derived, err := branchpkg.DerivedAppName(base, branch)
	if err != nil {
		t.Fatalf("derive branch: %v", err)
	}
	meta := branchMeta{
		BaseApp:         base,
		Branch:          branch,
		ShadowRepo:      "/tmp/repo",
		BaseSnapshotRef: "branch-base-1",
	}
	if err := writeBranchMetaAt(homeVolumeBaseDir, derived, meta); err != nil {
		t.Fatalf("write branch meta: %v", err)
	}
	if err := os.MkdirAll(homeVolumeConfigForApp(base).BaseDir, 0o755); err != nil {
		t.Fatalf("create base dir: %v", err)
	}

	state := server.State{Ports: map[string]int{base: 8080, derived: 8081}}
	if _, err := deleteApp("viberun-"+base, base, &state, false); err != nil {
		t.Fatalf("delete app: %v", err)
	}

	if _, err := os.Stat(homeVolumeConfigForApp(derived).BaseDir); !os.IsNotExist(err) {
		t.Fatalf("expected branch volume removed, got %v", err)
	}
	if _, ok := state.Ports[derived]; ok {
		t.Fatalf("expected branch removed from state")
	}
}
