// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	DefaultHost   string            `json:"default_host"`
	AgentProvider string            `json:"agent_provider"`
	Hosts         map[string]string `json:"hosts"`
}

func Load() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{Hosts: map[string]string{}}, path, nil
		}
		return Config{}, path, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, path, err
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}

	return cfg, path, nil
}

func Save(path string, cfg Config) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func configPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		var err error
		configHome, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(configHome, "viberun", "config.json"), nil
}
