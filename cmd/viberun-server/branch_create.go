// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	branchpkg "github.com/shayne/viberun/internal/branch"
	"github.com/shayne/viberun/internal/proxy"
)

const shadowGitBaseDir = "/var/lib/viberun/git"

func validateBranchCreateArgs(base string, branch string) (string, string, string, error) {
	baseNorm, err := proxy.NormalizeAppName(base)
	if err != nil {
		return "", "", "", err
	}
	branchNorm, err := branchpkg.NormalizeBranchName(branch)
	if err != nil {
		return "", "", "", err
	}
	derived, err := branchpkg.DerivedAppName(baseNorm, branchNorm)
	if err != nil {
		return "", "", "", err
	}
	return baseNorm, branchNorm, derived, nil
}

func createBranchEnv(base string, branch string) (branchMeta, error) {
	baseApp, branchName, derived, err := validateBranchCreateArgs(base, branch)
	if err != nil {
		return branchMeta{}, err
	}
	if _, ok, err := readBranchMetaAt(homeVolumeBaseDir, derived); err != nil {
		return branchMeta{}, err
	} else if ok {
		return branchMeta{}, fmt.Errorf("branch already exists: %s", derived)
	}
	branchCfg := homeVolumeConfigForApp(derived)
	if _, err := os.Stat(branchCfg.FilePath); err == nil {
		return branchMeta{}, fmt.Errorf("branch already exists: %s", derived)
	}
	baseContainer := fmt.Sprintf("viberun-%s", baseApp)
	if exists, err := containerExists(baseContainer); err != nil {
		return branchMeta{}, err
	} else if !exists {
		return branchMeta{}, fmt.Errorf("base app container does not exist")
	}
	baseCfg, ok, err := ensureHomeVolume(baseApp, false)
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("base app volume does not exist")
		}
		return branchMeta{}, err
	}
	stamp := time.Now().UTC().Format("20060102150405")
	tag := fmt.Sprintf("branch-base-%s", stamp)
	if err := snapshotContainer(baseContainer, baseCfg, tag); err != nil {
		return branchMeta{}, err
	}
	branchCfg, _, err = ensureHomeVolume(derived, true)
	if err != nil {
		return branchMeta{}, err
	}
	snapshotPath := snapshotPathForTag(baseCfg, tag)
	if err := copySnapshotToHome(snapshotPath, branchCfg.MountDir); err != nil {
		return branchMeta{}, err
	}
	if _, _, err := ensureHomeVolume(derived, false); err != nil {
		return branchMeta{}, err
	}
	meta := branchMeta{
		BaseApp:         baseApp,
		Branch:          branchName,
		CreatedAt:       time.Now().UTC(),
		BaseSnapshotRef: tag,
		ShadowRepo:      filepath.Join(shadowGitBaseDir, baseApp+".git"),
	}
	if err := writeBranchMetaAt(homeVolumeBaseDir, derived, meta); err != nil {
		return branchMeta{}, err
	}
	return meta, nil
}

func copySnapshotToHome(snapshotPath string, destHome string) error {
	if strings.TrimSpace(snapshotPath) == "" || strings.TrimSpace(destHome) == "" {
		return fmt.Errorf("snapshot and dest are required")
	}
	entries, err := os.ReadDir(destHome)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(destHome, entry.Name())); err != nil {
			return err
		}
	}
	cmd := exec.Command("tar", "-C", snapshotPath, "-cf", "-", ".")
	cmd2 := exec.Command("tar", "-C", destHome, "-xf", "-")
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

func isBranchAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "branch already exists")
}
