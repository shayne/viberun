// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shayne/viberun/internal/proxy"
	"github.com/shayne/viberun/internal/server"
)

type branchCommand struct {
	action string
	base   string
	branch string
}

func parseBranchCommand(args []string) (branchCommand, error) {
	if len(args) < 2 {
		return branchCommand{}, newUsageError("Usage: viberun-server branch <list|create|delete|apply> <app> [branch]")
	}
	return branchCommand{
		action: strings.TrimSpace(args[0]),
		base:   strings.TrimSpace(args[1]),
		branch: strings.TrimSpace(strings.Join(args[2:], " ")),
	}, nil
}

func handleBranchCommand(args []string) error {
	cmd, err := parseBranchCommand(args)
	if err != nil {
		return err
	}
	switch cmd.action {
	case "list":
		return runBranchList(cmd.base)
	case "create":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch create <app> <branch>")
		}
		meta, err := createBranchEnv(cmd.base, cmd.branch)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Created branch %s for %s\n", meta.Branch, meta.BaseApp)
		return nil
	case "delete", "rm":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch delete <app> <branch>")
		}
		return deleteBranchEnv(cmd.base, cmd.branch)
	case "apply":
		if cmd.branch == "" {
			return newUsageError("Usage: viberun-server branch apply <app> <branch>")
		}
		return applyBranch(cmd.base, cmd.branch)
	default:
		return newUsageError("Usage: viberun-server branch <list|create|delete|apply> <app> [branch]")
	}
}

func runBranchList(base string) error {
	metas, err := listBranchMetas(base)
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		baseApp, err := proxy.NormalizeAppName(base)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "No branches found for %s\n", baseApp)
		return nil
	}
	fmt.Fprintf(os.Stdout, "Branches for %s:\n", metas[0].BaseApp)
	for _, meta := range metas {
		line := meta.Branch
		if !meta.CreatedAt.IsZero() {
			line = fmt.Sprintf("%s (created %s)", line, meta.CreatedAt.UTC().Format(time.RFC3339))
		}
		fmt.Fprintf(os.Stdout, "  %s\n", line)
	}
	return nil
}

func listBranchMetas(base string) ([]branchMeta, error) {
	baseApp, err := proxy.NormalizeAppName(base)
	if err != nil {
		return nil, err
	}
	metas, err := listBranchMetasAt(homeVolumeBaseDir, baseApp)
	if err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return nil, nil
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Branch < metas[j].Branch
	})
	return metas, nil
}

func deleteBranchEnv(base string, branch string) error {
	baseApp, branchName, derived, err := validateBranchCreateArgs(base, branch)
	if err != nil {
		return err
	}
	state, statePath, err := server.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load server state: %w", err)
	}
	stateDirty := false
	containerName := fmt.Sprintf("viberun-%s", derived)
	exists, err := containerExists(containerName)
	if err != nil {
		return err
	}
	deletedState, err := deleteApp(containerName, derived, &state, exists)
	if err != nil {
		return err
	}
	if deletedState {
		stateDirty = true
	}
	if err := persistState(statePath, &state, &stateDirty); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Deleted branch %s for %s\n", branchName, baseApp)
	return nil
}
