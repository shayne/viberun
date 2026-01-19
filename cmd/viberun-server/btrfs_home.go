// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	homeVolumeBaseDir = "/var/lib/viberun/apps"
	homeVolumeSize    = int64(1_000_000_000_000) // 1 TB sparse file
)

var (
	cachedImageUser string
	cachedImageUID  int
	cachedImageGID  int
)

type homeVolumeConfig struct {
	App          string
	BaseDir      string
	FilePath     string
	RootMountDir string
	MountDir     string
	SnapshotsDir string
	MetaPath     string
}

type homeVolumeMeta struct {
	App        string    `json:"app"`
	FilePath   string    `json:"file_path"`
	LoopDevice string    `json:"loop_device"`
	MountedAt  time.Time `json:"mounted_at"`
}

func homeVolumeConfigForApp(app string) homeVolumeConfig {
	safe := sanitizeHostRPCName(app)
	baseDir := filepath.Join(homeVolumeBaseDir, safe)
	return homeVolumeConfig{
		App:          app,
		BaseDir:      baseDir,
		FilePath:     filepath.Join(baseDir, "home.btrfs"),
		RootMountDir: filepath.Join(baseDir, "btrfs"),
		MountDir:     filepath.Join(baseDir, "mount"),
		SnapshotsDir: filepath.Join(baseDir, "snapshots"),
		MetaPath:     filepath.Join(baseDir, "meta.json"),
	}
}

func ensureHomeVolume(app string, create bool) (homeVolumeConfig, bool, error) {
	cfg := homeVolumeConfigForApp(app)
	if err := ensureBtrfsTools(); err != nil {
		return cfg, false, err
	}
	uid, gid, err := containerUserIDs(defaultImage)
	if err != nil {
		return cfg, false, err
	}
	if err := os.MkdirAll(cfg.BaseDir, 0o755); err != nil {
		return cfg, false, err
	}
	exists := true
	if _, err := os.Stat(cfg.FilePath); err != nil {
		if os.IsNotExist(err) {
			exists = false
		} else {
			return cfg, false, err
		}
	}
	created, err := ensureSparseFile(cfg.FilePath, homeVolumeSize, create)
	if err != nil {
		return cfg, false, err
	}
	if !exists && !create {
		return cfg, false, nil
	}
	loop, err := ensureLoopDevice(cfg.FilePath)
	if err != nil {
		return cfg, false, err
	}
	if err := ensureBtrfsFilesystem(loop, created); err != nil {
		return cfg, false, err
	}
	if err := ensureRootMount(loop, cfg.RootMountDir); err != nil {
		return cfg, false, err
	}
	if err := ensureSubvolumes(cfg); err != nil {
		return cfg, false, err
	}
	if err := ensureSubvolumeMounted(loop, "@home", cfg.MountDir); err != nil {
		return cfg, false, err
	}
	if err := ensureSubvolumeMounted(loop, "@snapshots", cfg.SnapshotsDir); err != nil {
		return cfg, false, err
	}
	if created || !homeOwnedBy(cfg.MountDir, uid, gid) {
		if err := runHostCommand("chown", fmt.Sprintf("%d:%d", uid, gid), cfg.MountDir).Run(); err != nil {
			return cfg, false, fmt.Errorf("failed to set ownership on %s: %w", cfg.MountDir, err)
		}
		if err := runHostCommand("chmod", "0755", cfg.MountDir).Run(); err != nil {
			return cfg, false, fmt.Errorf("failed to set permissions on %s: %w", cfg.MountDir, err)
		}
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, "app"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, ".local", "services"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, ".local", "logs"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := writeHomeVolumeMeta(cfg, loop); err != nil {
		return cfg, false, err
	}
	return cfg, true, nil
}

func ensureSparseFile(path string, size int64, create bool) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.Size() == 0 && size > 0 {
			return false, fmt.Errorf("btrfs file exists but is empty: %s", path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if !create {
		return false, nil
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return false, err
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func ensureBtrfsTools() error {
	required := []string{"btrfs", "mkfs.btrfs", "losetup", "mount", "umount"}
	for _, name := range required {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("missing %s on host; rerun bootstrap to install btrfs-progs", name)
		}
	}
	if _, err := exec.LookPath("sudo"); err != nil && os.Geteuid() != 0 {
		return fmt.Errorf("sudo is required to manage btrfs volumes; rerun bootstrap with sudo access")
	}
	return nil
}

func ensureLoopDevice(path string) (string, error) {
	loop, err := findLoopDevice(path)
	if err != nil {
		return "", err
	}
	if loop != "" {
		return loop, nil
	}
	cmd := runHostCommand("losetup", "--find", "--show", "--nooverlap", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to attach loop device: %s", strings.TrimSpace(string(out)))
	}
	loop = strings.TrimSpace(string(out))
	if loop == "" {
		return "", fmt.Errorf("failed to attach loop device for %s", path)
	}
	return loop, nil
}

func findLoopDevice(path string) (string, error) {
	cmd := runHostCommand("losetup", "-j", path)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(exitErr.Stderr) > 0 {
				return "", fmt.Errorf("failed to query loop devices: %s", strings.TrimSpace(string(exitErr.Stderr)))
			}
		}
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var devices []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) > 0 && strings.HasPrefix(parts[0], "/dev/") {
			devices = append(devices, parts[0])
		}
	}
	if len(devices) == 0 {
		return "", nil
	}
	if len(devices) > 1 {
		return "", fmt.Errorf("multiple loop devices attached to %s: %s", path, strings.Join(devices, ", "))
	}
	return devices[0], nil
}

func ensureBtrfsFilesystem(loop string, created bool) error {
	if created {
		if err := runHostCommandOutput("mkfs.btrfs", "-f", loop); err != nil {
			return err
		}
		return nil
	}
	if ok, err := isBtrfsDevice(loop); err == nil && ok {
		return nil
	}
	cmd := runHostCommand("btrfs", "filesystem", "show", loop)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("loop device is not a btrfs filesystem: %s", loop)
	}
	return nil
}

func isBtrfsDevice(loop string) (bool, error) {
	if _, err := exec.LookPath("blkid"); err != nil {
		return false, err
	}
	cmd := runHostCommand("blkid", "-o", "value", "-s", "TYPE", loop)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "btrfs", nil
}

func ensureRootMount(loop string, root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if info, ok := mountInfoForTarget(root); ok {
		if info.Source != loop {
			return fmt.Errorf("root mount %s already in use by %s", root, info.Source)
		}
		return nil
	}
	return runHostCommandOutput("mount", "-t", "btrfs", loop, root)
}

func ensureSubvolumes(cfg homeVolumeConfig) error {
	paths := []string{
		filepath.Join(cfg.RootMountDir, "@home"),
		filepath.Join(cfg.RootMountDir, "@snapshots"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := runHostCommandOutput("btrfs", "subvolume", "create", path); err != nil {
			return err
		}
	}
	return nil
}

func ensureSubvolumeMounted(loop string, subvol string, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if info, ok := mountInfoForTarget(target); ok {
		if info.Source != loop {
			return fmt.Errorf("mount %s already in use by %s", target, info.Source)
		}
		if !strings.Contains(info.Options, "subvol="+subvol) && !strings.Contains(info.Options, "subvol=/"+subvol) {
			return fmt.Errorf("mount %s is not using subvol %s", target, subvol)
		}
		return nil
	}
	return runHostCommandOutput("mount", "-t", "btrfs", "-o", "subvol="+subvol, loop, target)
}

func writeHomeVolumeMeta(cfg homeVolumeConfig, loop string) error {
	meta := homeVolumeMeta{
		App:        cfg.App,
		FilePath:   cfg.FilePath,
		LoopDevice: loop,
		MountedAt:  time.Now().UTC(),
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.MetaPath, data, 0o644)
}

func deleteHomeVolume(app string) error {
	cfg := homeVolumeConfigForApp(app)
	if _, err := os.Stat(cfg.BaseDir); os.IsNotExist(err) {
		return nil
	}
	if info, ok := mountInfoForTarget(cfg.MountDir); ok {
		_ = runHostCommand("umount", info.Target).Run()
	}
	if info, ok := mountInfoForTarget(cfg.SnapshotsDir); ok {
		_ = runHostCommand("umount", info.Target).Run()
	}
	if info, ok := mountInfoForTarget(cfg.RootMountDir); ok {
		_ = runHostCommand("umount", info.Target).Run()
	}
	loop, _ := findLoopDevice(cfg.FilePath)
	if loop != "" {
		_ = runHostCommand("losetup", "-d", loop).Run()
	}
	return os.RemoveAll(cfg.BaseDir)
}

type mountInfo struct {
	Source  string
	Target  string
	Fstype  string
	Options string
}

func mountInfoForTarget(target string) (mountInfo, bool) {
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return mountInfo{}, false
	}
	for _, line := range strings.Split(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[1] == target {
			return mountInfo{
				Source:  fields[0],
				Target:  fields[1],
				Fstype:  fields[2],
				Options: fields[3],
			}, true
		}
	}
	return mountInfo{}, false
}

func runHostCommand(name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(name, args...)
	}
	sudoArgs := append([]string{"-n", name}, args...)
	return exec.Command("sudo", sudoArgs...)
}

func runHostCommandOutput(name string, args ...string) error {
	cmd := runHostCommand(name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return fmt.Errorf("failed to run %s: %w", name, err)
	}
	return fmt.Errorf("failed to run %s: %w: %s", name, err, output)
}

func ensureOwnedDir(path string, uid int, gid int) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := runHostCommand("chown", fmt.Sprintf("%d:%d", uid, gid), path).Run(); err != nil {
		return fmt.Errorf("failed to set ownership on %s: %w", path, err)
	}
	return nil
}

func homeOwnedBy(path string, uid int, gid int) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(stat.Uid) == uid && int(stat.Gid) == gid
}

func containerUserIDs(image string) (int, int, error) {
	if strings.TrimSpace(image) == "" {
		return 0, 0, fmt.Errorf("image name required")
	}
	if cachedImageUser == image && cachedImageUID != 0 && cachedImageGID != 0 {
		return cachedImageUID, cachedImageGID, nil
	}
	uid, err := containerUserID(image, "-u")
	if err != nil {
		return 0, 0, err
	}
	gid, err := containerUserID(image, "-g")
	if err != nil {
		return 0, 0, err
	}
	cachedImageUser = image
	cachedImageUID = uid
	cachedImageGID = gid
	return uid, gid, nil
}

func containerUserID(image string, flag string) (int, error) {
	cmd := exec.Command("docker", "run", "--rm", "--entrypoint", "id", image, flag)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to resolve user id for %s (%s): %w", image, flag, err)
	}
	value := strings.TrimSpace(string(out))
	id, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid id output %q for %s (%s)", value, image, flag)
	}
	return id, nil
}

func snapshotPathForTag(cfg homeVolumeConfig, tag string) string {
	return filepath.Join(cfg.SnapshotsDir, tag)
}

func snapshotTag(name string) (string, bool) {
	tag := strings.TrimSpace(name)
	if tag == "" || strings.Contains(tag, "/") || strings.Contains(tag, string(filepath.Separator)) || strings.Contains(tag, ":") {
		return "", false
	}
	return tag, true
}

func parseSnapshotTag(tag string) (int, bool) {
	normalized := strings.TrimSpace(strings.ToLower(tag))
	if normalized == "" || !strings.HasPrefix(normalized, "v") {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimPrefix(normalized, "v"))
	if err != nil || value < 1 {
		return 0, false
	}
	return value, true
}
