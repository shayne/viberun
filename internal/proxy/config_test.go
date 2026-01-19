// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingReturnsDefaults(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proxy.toml")
	t.Setenv("VIBERUN_PROXY_CONFIG_PATH", path)

	cfg, gotPath, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if gotPath != path {
		t.Fatalf("expected path %s, got %s", path, gotPath)
	}
	if cfg.Enabled {
		t.Fatalf("expected disabled by default")
	}
	if cfg.AdminAddr != defaultAdminAddr {
		t.Fatalf("expected default admin addr, got %q", cfg.AdminAddr)
	}
	if cfg.CaddyContainer != defaultCaddyContainer {
		t.Fatalf("expected default container, got %q", cfg.CaddyContainer)
	}
	if cfg.ProxyImage != defaultProxyImage {
		t.Fatalf("expected default proxy image, got %q", cfg.ProxyImage)
	}
	if cfg.DefaultAccess != defaultAccessMode {
		t.Fatalf("expected default access mode, got %q", cfg.DefaultAccess)
	}
	if cfg.Auth.CookieName != defaultAuthCookieName {
		t.Fatalf("expected default auth cookie name, got %q", cfg.Auth.CookieName)
	}
	if cfg.Env.PublicURL != defaultEnvPublicURL {
		t.Fatalf("expected default public url env, got %q", cfg.Env.PublicURL)
	}
	if cfg.Env.PublicDomain != defaultEnvDomain {
		t.Fatalf("expected default public domain env, got %q", cfg.Env.PublicDomain)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proxy.toml")
	cfg := Config{
		Enabled:    true,
		BaseDomain: "example.com",
		PublicIP:   "1.2.3.4",
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}

	loaded, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if !loaded.Enabled {
		t.Fatalf("expected enabled")
	}
	if loaded.BaseDomain != "example.com" {
		t.Fatalf("expected base domain, got %q", loaded.BaseDomain)
	}
	if loaded.PublicIP != "1.2.3.4" {
		t.Fatalf("expected public ip, got %q", loaded.PublicIP)
	}
	if loaded.AdminAddr == "" || loaded.CaddyContainer == "" {
		t.Fatalf("expected defaults to be applied")
	}
	if loaded.ProxyImage == "" {
		t.Fatalf("expected proxy defaults to be applied")
	}
}
