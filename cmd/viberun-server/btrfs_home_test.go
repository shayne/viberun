// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestHomeVolumeConfigForApp(t *testing.T) {
	cfg := homeVolumeConfigForApp("My App")
	if cfg.BaseDir != "/var/lib/viberun/apps/my_app" {
		t.Fatalf("unexpected base dir: %s", cfg.BaseDir)
	}
	if cfg.FilePath != "/var/lib/viberun/apps/my_app/home.btrfs" {
		t.Fatalf("unexpected file path: %s", cfg.FilePath)
	}
	if cfg.MountDir != "/var/lib/viberun/apps/my_app/mount" {
		t.Fatalf("unexpected mount dir: %s", cfg.MountDir)
	}
	if cfg.SnapshotsDir != "/var/lib/viberun/apps/my_app/snapshots" {
		t.Fatalf("unexpected snapshots dir: %s", cfg.SnapshotsDir)
	}
}

func TestSnapshotPathForTag(t *testing.T) {
	cfg := homeVolumeConfigForApp("app")
	got := snapshotPathForTag(cfg, "v5")
	want := "/var/lib/viberun/apps/app/snapshots/v5"
	if got != want {
		t.Fatalf("snapshot path=%s want %s", got, want)
	}
}
