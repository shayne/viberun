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
		cmd := exec.Command("docker", "pause", containerName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		paused = true
	}
	defer func() {
		if paused {
			_ = exec.Command("docker", "unpause", containerName).Run()
		}
	}()
	_ = exec.Command("sync").Run()
	cmd := runHostCommand("btrfs", "subvolume", "snapshot", "-r", cfg.MountDir, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
		_ = runHostCommand("umount", info.Target).Run()
	}
	rootPath := filepath.Join(cfg.RootMountDir, "@home")
	if _, err := os.Stat(rootPath); err == nil {
		cmd := runHostCommand("btrfs", "subvolume", "delete", rootPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	cmd := runHostCommand("btrfs", "subvolume", "snapshot", snapshot, rootPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
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
