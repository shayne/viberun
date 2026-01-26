// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func applyBranch(base string, branch string) error {
	baseApp, branchName, derived, err := validateBranchCreateArgs(base, branch)
	if err != nil {
		return err
	}
	meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, derived)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("branch not found: %s", branchName)
	}
	if meta.BaseApp != baseApp || meta.Branch != branchName {
		return fmt.Errorf("branch metadata mismatch")
	}
	if strings.TrimSpace(meta.BaseSnapshotRef) == "" {
		return fmt.Errorf("branch metadata missing base snapshot; re-create the branch")
	}
	baseCfg, ok, err := ensureHomeVolume(baseApp, false)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("base app volume missing")
		}
		return err
	}
	branchCfg, ok, err := ensureHomeVolume(derived, false)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("branch volume missing")
		}
		return err
	}
	baseContainer := fmt.Sprintf("viberun-%s", baseApp)
	if exists, err := containerExists(baseContainer); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("base app container missing")
	}
	if _, err := createSnapshot(baseContainer, baseApp); err != nil {
		return err
	}
	repo := strings.TrimSpace(meta.ShadowRepo)
	if repo == "" {
		repo = filepath.Join(shadowGitBaseDir, baseApp+".git")
	}
	if repo == "" {
		return fmt.Errorf("shadow repo path missing; re-create the branch")
	}
	if meta.ShadowRepo == "" {
		meta.ShadowRepo = repo
		_ = writeBranchMetaAt(homeVolumeBaseDir, derived, meta)
	}
	if err := ensureShadowRepo(repo); err != nil {
		return err
	}
	baseSnapshot := snapshotPathForTag(baseCfg, meta.BaseSnapshotRef)
	baseSnapshotApp := filepath.Join(baseSnapshot, "app")
	if _, err := os.Stat(baseSnapshotApp); err != nil {
		return fmt.Errorf("base snapshot not found: %w", err)
	}
	if err := buildShadowCommits(repo, baseSnapshotApp, filepath.Join(baseCfg.MountDir, "app"), filepath.Join(branchCfg.MountDir, "app"), branchName); err != nil {
		return err
	}
	if err := gitMergeIntoBranch(repo, branchName, "main", filepath.Join(branchCfg.MountDir, "app")); err != nil {
		return err
	}
	if err := gitFastForward(repo, "main", branchName); err != nil {
		return err
	}
	return syncAppDir(filepath.Join(branchCfg.MountDir, "app"), filepath.Join(baseCfg.MountDir, "app"))
}

func applyBranchForApp(app string) error {
	meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, app)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("branch metadata not found")
	}
	return applyBranch(meta.BaseApp, meta.Branch)
}

func ensureShadowRepo(repo string) error {
	if strings.TrimSpace(repo) == "" {
		return fmt.Errorf("shadow repo path required")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is required but was not found in PATH")
	}
	if _, err := os.Stat(repo); err == nil {
		_, err := runGitDirCapture(repo, "", "rev-parse", "--git-dir")
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(repo), 0o755); err != nil {
		return err
	}
	if err := runGitCommand("", "init", "--bare", repo); err != nil {
		return err
	}
	if err := runGitDir(repo, "", "config", "user.name", "viberun"); err != nil {
		return err
	}
	if err := runGitDir(repo, "", "config", "user.email", "viberun@localhost"); err != nil {
		return err
	}
	return nil
}

func buildShadowCommits(repo string, baseSnapshot string, baseAppDir string, branchAppDir string, branchName string) error {
	tmp, err := os.MkdirTemp("", "viberun-shadow-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := runGitCommandSafe(tmp, tmp, "init"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "config", "user.name", "viberun"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "config", "user.email", "viberun@localhost"); err != nil {
		return err
	}
	if err := syncAppDir(baseSnapshot, tmp); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "add", "-A"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "commit", "-m", "base", "--allow-empty"); err != nil {
		return err
	}
	baseRef, err := runGitCommandCaptureSafe(tmp, tmp, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	baseRef = strings.TrimSpace(baseRef)
	if err := runGitCommandSafe(tmp, tmp, "checkout", "-B", "main"); err != nil {
		return err
	}
	if err := syncAppDir(baseAppDir, tmp); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "add", "-A"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "commit", "-m", "main", "--allow-empty"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "checkout", "-B", branchName, baseRef); err != nil {
		return err
	}
	if err := syncAppDir(branchAppDir, tmp); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "add", "-A"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "commit", "-m", "branch", "--allow-empty"); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "remote", "add", "shadow", repo); err != nil {
		return err
	}
	if err := runGitCommandSafe(tmp, tmp, "push", "--force", "shadow", "main:refs/heads/main"); err != nil {
		return err
	}
	return runGitCommandSafe(tmp, tmp, "push", "--force", "shadow", fmt.Sprintf("HEAD:refs/heads/%s", branchName))
}

func gitMergeIntoBranch(repo string, branch string, base string, worktree string) error {
	if err := runGitDir(repo, worktree, "checkout", "-f", branch); err != nil {
		return err
	}
	out, err := runGitDirCapture(repo, worktree, "merge", "--no-edit", base)
	if err != nil {
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			trimmed = err.Error()
		}
		return errors.New(trimmed)
	}
	return nil
}

func gitFastForward(repo string, main string, branch string) error {
	return runGitDir(repo, "", "branch", "-f", main, branch)
}

func syncAppDir(src string, dest string) error {
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dest) == "" {
		return fmt.Errorf("source and destination required")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	cmd := exec.Command("tar", "-C", src, "--exclude=.git", "-cf", "-", ".")
	cmd2 := exec.Command("tar", "-C", dest, "-xf", "-")
	r, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd2.Stdin = r
	if err := cmd2.Start(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return cmd2.Wait()
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%s", trimmed)
	}
	return nil
}

func runGitCommandCapture(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(out), nil
}

func runGitCommandSafe(dir string, safeDir string, args ...string) error {
	cmdArgs := append([]string{}, args...)
	if strings.TrimSpace(safeDir) != "" {
		cmdArgs = append([]string{"-c", fmt.Sprintf("safe.directory=%s", safeDir)}, cmdArgs...)
	}
	return runGitCommand(dir, cmdArgs...)
}

func runGitCommandCaptureSafe(dir string, safeDir string, args ...string) (string, error) {
	cmdArgs := append([]string{}, args...)
	if strings.TrimSpace(safeDir) != "" {
		cmdArgs = append([]string{"-c", fmt.Sprintf("safe.directory=%s", safeDir)}, cmdArgs...)
	}
	return runGitCommandCapture(dir, cmdArgs...)
}

func runGitDir(repo string, worktree string, args ...string) error {
	cmdArgs := []string{"--git-dir", repo}
	if strings.TrimSpace(worktree) != "" {
		cmdArgs = append(cmdArgs, "--work-tree", worktree)
	}
	cmdArgs = append(cmdArgs, args...)
	return runGitCommand("", cmdArgs...)
}

func runGitDirCapture(repo string, worktree string, args ...string) (string, error) {
	cmdArgs := []string{"--git-dir", repo}
	if strings.TrimSpace(worktree) != "" {
		cmdArgs = append(cmdArgs, "--work-tree", worktree)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(out), nil
}
