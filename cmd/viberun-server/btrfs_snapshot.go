// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shayne/viberun/internal/hostcmd"
)

func snapshotContainer(containerName string, cfg homeVolumeConfig, tag string) error {
	if strings.TrimSpace(containerName) == "" {
		return fmt.Errorf("container name required for snapshot")
	}
	if strings.TrimSpace(tag) == "" {
		return fmt.Errorf("snapshot tag required")
	}
	dest := snapshotPathForTag(cfg, tag)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("snapshot already exists: %s", tag)
	}
	paused := false
	if running, _ := containerRunning(containerName); running {
		if err := runDockerCommandOutput("pause", containerName); err != nil {
			return err
		}
		paused = true
	}
	defer func() {
		if paused {
			_ = runDockerCommandOutput("unpause", containerName)
		}
	}()
	_ = exec.Command("sync").Run()
	return hostcmd.RunOutput("btrfs", "subvolume", "snapshot", "-r", cfg.MountDir, dest)
}

func restoreHomeVolume(cfg homeVolumeConfig, tag string) error {
	if _, ok := snapshotTag(tag); !ok {
		return fmt.Errorf("invalid snapshot tag: %s", tag)
	}
	snapshot := snapshotPathForTag(cfg, tag)
	if _, err := os.Stat(snapshot); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("snapshot not found: %s", tag)
		}
		return err
	}
	if info, ok := mountInfoForTarget(cfg.MountDir); ok {
		_ = hostcmd.Run("umount", info.Target).Run()
	}
	rootPath := filepath.Join(cfg.RootMountDir, "@home")
	if _, err := os.Stat(rootPath); err == nil {
		if err := hostcmd.RunOutput("btrfs", "subvolume", "delete", rootPath); err != nil {
			return err
		}
	}
	if err := hostcmd.RunOutput("btrfs", "subvolume", "snapshot", snapshot, rootPath); err != nil {
		return err
	}
	loop, err := findLoopDevice(cfg.FilePath)
	if err != nil {
		return err
	}
	if loop == "" {
		return fmt.Errorf("loop device not found for %s", cfg.FilePath)
	}
	return ensureSubvolumeMounted(loop, "@home", cfg.MountDir)
}
