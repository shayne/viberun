// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const branchMetaFilename = "branch.json"

type branchMeta struct {
	BaseApp         string    `json:"base_app"`
	Branch          string    `json:"branch"`
	CreatedAt       time.Time `json:"created_at"`
	BaseSnapshotRef string    `json:"base_snapshot_ref"`
	ShadowRepo      string    `json:"shadow_repo"`
}

func branchMetaPathAt(baseDir, app string) string {
	return filepath.Join(baseDir, sanitizeHostRPCName(app), branchMetaFilename)
}

func writeBranchMetaAt(baseDir, app string, meta branchMeta) error {
	path := branchMetaPathAt(baseDir, app)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readBranchMetaAt(baseDir, app string) (branchMeta, bool, error) {
	path := branchMetaPathAt(baseDir, app)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return branchMeta{}, false, nil
		}
		return branchMeta{}, false, err
	}
	var meta branchMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return branchMeta{}, false, err
	}
	return meta, true, nil
}

func listBranchMetasAt(baseDir, baseApp string) ([]branchMeta, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]branchMeta, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, ok, err := readBranchMetaAt(baseDir, entry.Name())
		if err != nil {
			return nil, err
		}
		if !ok || meta.BaseApp != baseApp {
			continue
		}
		out = append(out, meta)
	}
	return out, nil
}
