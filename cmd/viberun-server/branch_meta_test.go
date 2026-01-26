// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBranchMetaLoadAndList(t *testing.T) {
	base := t.TempDir()
	meta := branchMeta{BaseApp: "app", Branch: "feature", ShadowRepo: "/tmp/repo", BaseSnapshotRef: "branch-base-1"}
	if err := writeBranchMetaAt(base, "app--feature", meta); err != nil {
		t.Fatalf("writeBranchMetaAt: %v", err)
	}
	list, err := listBranchMetasAt(base, "app")
	if err != nil {
		t.Fatalf("listBranchMetasAt: %v", err)
	}
	if len(list) != 1 || list[0].Branch != "feature" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if _, err := os.Stat(filepath.Join(base, "app--feature", "branch.json")); err != nil {
		t.Fatalf("missing branch.json: %v", err)
	}
}
