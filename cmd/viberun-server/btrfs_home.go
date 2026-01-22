// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shayne/viberun/internal/hostcmd"
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
	AclMarker    string
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
		AclMarker:    filepath.Join(baseDir, "acl.applied"),
	}
}

func ensureHomeVolume(app string, create bool) (homeVolumeConfig, bool, error) {
	cfg := homeVolumeConfigForApp(app)
	if err := ensureBtrfsTools(); err != nil {
		return cfg, false, err
	}
	if err := ensureAclTools(); err != nil {
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
		if err := hostcmd.Run("chown", fmt.Sprintf("%d:%d", uid, gid), cfg.MountDir).Run(); err != nil {
			return cfg, false, fmt.Errorf("failed to set ownership on %s: %w", cfg.MountDir, err)
		}
		if err := hostcmd.Run("chmod", "0755", cfg.MountDir).Run(); err != nil {
			return cfg, false, fmt.Errorf("failed to set permissions on %s: %w", cfg.MountDir, err)
		}
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, "app"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, ".local"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, ".local", "services"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureOwnedDir(filepath.Join(cfg.MountDir, ".local", "logs"), uid, gid); err != nil {
		return cfg, false, err
	}
	if err := ensureHomeACL(cfg, uid, gid); err != nil {
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
	if os.Geteuid() != 0 {
		return fmt.Errorf("viberun-server must run as root; re-run with sudo")
	}
	required := []string{"btrfs", "mkfs.btrfs", "losetup", "mount", "umount"}
	for _, name := range required {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("missing %s on host; rerun bootstrap to install btrfs-progs", name)
		}
	}
	return nil
}

func ensureAclTools() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("viberun-server must run as root; re-run with sudo")
	}
	if _, err := exec.LookPath("setfacl"); err != nil {
		return fmt.Errorf("missing setfacl on host; rerun bootstrap to install acl")
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
	cmd := hostcmd.Run("losetup", "--find", "--show", "--nooverlap", path)
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
	cmd := hostcmd.Run("losetup", "-j", path)
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
		if err := hostcmd.RunOutput("mkfs.btrfs", "-f", loop); err != nil {
			return err
		}
		return nil
	}
	if ok, err := isBtrfsDevice(loop); err == nil && ok {
		return nil
	}
	cmd := hostcmd.Run("btrfs", "filesystem", "show", loop)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("loop device is not a btrfs filesystem: %s", loop)
	}
	return nil
}

func isBtrfsDevice(loop string) (bool, error) {
	if _, err := exec.LookPath("blkid"); err != nil {
		return false, err
	}
	cmd := hostcmd.Run("blkid", "-o", "value", "-s", "TYPE", loop)
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
	return hostcmd.RunOutput("mount", "-t", "btrfs", loop, root)
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
		if err := hostcmd.RunOutput("btrfs", "subvolume", "create", path); err != nil {
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
	return hostcmd.RunOutput("mount", "-t", "btrfs", "-o", "subvol="+subvol, loop, target)
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
		_ = hostcmd.Run("umount", info.Target).Run()
	}
	if info, ok := mountInfoForTarget(cfg.SnapshotsDir); ok {
		_ = hostcmd.Run("umount", info.Target).Run()
	}
	if info, ok := mountInfoForTarget(cfg.RootMountDir); ok {
		_ = hostcmd.Run("umount", info.Target).Run()
	}
	loop, _ := findLoopDevice(cfg.FilePath)
	if loop != "" {
		_ = hostcmd.Run("losetup", "-d", loop).Run()
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

func ensureOwnedDir(path string, uid int, gid int) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := hostcmd.Run("chown", fmt.Sprintf("%d:%d", uid, gid), path).Run(); err != nil {
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

func ensureHomeACL(cfg homeVolumeConfig, uid int, gid int) error {
	if err := setAccessACL(cfg.MountDir, uid, gid); err != nil {
		return err
	}
	if err := setDefaultACL(cfg.MountDir, uid, gid); err != nil {
		return err
	}
	if cfg.AclMarker == "" {
		return nil
	}
	if _, err := os.Stat(cfg.AclMarker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := setAccessACLRecursive(cfg.MountDir, uid, gid); err != nil {
		return err
	}
	if err := setDefaultACLRecursive(cfg.MountDir, uid, gid); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.AclMarker, []byte(time.Now().Format(time.RFC3339)), 0o644); err != nil {
		return fmt.Errorf("failed to write acl marker: %w", err)
	}
	return nil
}

func aclSpec(uid int, gid int) string {
	return fmt.Sprintf("u:%d:rwx,g:%d:rwx,m::rwx", uid, gid)
}

func setAccessACL(path string, uid int, gid int) error {
	cmd := hostcmd.Run("setfacl", "-m", aclSpec(uid, gid), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set ACL on %s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}

func setDefaultACL(path string, uid int, gid int) error {
	cmd := hostcmd.Run("setfacl", "-d", "-m", aclSpec(uid, gid), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set default ACL on %s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}

func setAccessACLRecursive(path string, uid int, gid int) error {
	cmd := hostcmd.Run("setfacl", "-R", "-m", aclSpec(uid, gid), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set recursive ACL on %s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}

func setDefaultACLRecursive(root string, uid int, gid int) error {
	const batchSize = 128
	spec := aclSpec(uid, gid)
	batch := make([]string, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		args := append([]string{"-d", "-m", spec}, batch...)
		cmd := hostcmd.Run("setfacl", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to set default ACLs under %s: %s", root, strings.TrimSpace(string(out)))
		}
		batch = batch[:0]
		return nil
	}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		batch = append(batch, path)
		if len(batch) >= batchSize {
			return flush()
		}
		return nil
	}); err != nil {
		return err
	}
	return flush()
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
