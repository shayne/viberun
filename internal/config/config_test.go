// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := Config{
		DefaultHost:   "example.com",
		AgentProvider: "codex",
		Hosts: map[string]string{
			"dev": "dev.example.com",
		},
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	if filepath.Dir(path) == "" {
		t.Fatalf("expected config dir to be set")
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, loadedPath, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loadedPath != path {
		t.Fatalf("expected path %s, got %s", path, loadedPath)
	}
	if loaded.DefaultHost != cfg.DefaultHost {
		t.Fatalf("default host mismatch: %s != %s", loaded.DefaultHost, cfg.DefaultHost)
	}
	if loaded.AgentProvider != cfg.AgentProvider {
		t.Fatalf("agent provider mismatch: %s != %s", loaded.AgentProvider, cfg.AgentProvider)
	}
	if loaded.Hosts["dev"] != cfg.Hosts["dev"] {
		t.Fatalf("host alias mismatch: %s != %s", loaded.Hosts["dev"], cfg.Hosts["dev"])
	}
}
