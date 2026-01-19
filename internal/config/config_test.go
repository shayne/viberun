// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"os"
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

func TestLoadLegacyJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	legacyPath, err := legacyConfigPath()
	if err != nil {
		t.Fatalf("legacyConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := Config{
		DefaultHost:   "legacy.example.com",
		AgentProvider: "codex",
		Hosts: map[string]string{
			"legacy": "legacy.example.com",
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(legacyPath, data, 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	loaded, loadedPath, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
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
	if loaded.Hosts["legacy"] != cfg.Hosts["legacy"] {
		t.Fatalf("host alias mismatch: %s != %s", loaded.Hosts["legacy"], cfg.Hosts["legacy"])
	}
}

func TestRemoveConfigFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	legacyPath, err := legacyConfigPath()
	if err != nil {
		t.Fatalf("legacyConfigPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	if err := RemoveConfigFiles(); err != nil {
		t.Fatalf("RemoveConfigFiles: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config removed, got %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy removed, got %v", err)
	}
}
