// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	DefaultHost   string            `json:"default_host" toml:"default_host"`
	AgentProvider string            `json:"agent_provider" toml:"agent_provider"`
	Hosts         map[string]string `json:"hosts" toml:"hosts"`
}

func Load() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := loadToml(path)
	if err == nil {
		return cfg, path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Config{}, path, err
	}
	legacyPath, legacyErr := legacyConfigPath()
	if legacyErr != nil {
		return Config{Hosts: map[string]string{}}, path, nil
	}
	cfg, err = loadLegacyJSON(legacyPath)
	if err == nil {
		return cfg, path, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Config{Hosts: map[string]string{}}, path, nil
	}
	return Config{}, path, err
}

func Save(path string, cfg Config) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}
	data, err := toml.Marshal(cfg)
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

	return filepath.Join(configHome, "viberun", "config.toml"), nil
}

func legacyConfigPath() (string, error) {
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

func loadToml(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}
	return cfg, nil
}

func loadLegacyJSON(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]string{}
	}
	return cfg, nil
}

func RemoveConfigFiles() error {
	paths := []string{}
	if path, err := configPath(); err == nil {
		paths = append(paths, path)
	}
	if path, err := legacyConfigPath(); err == nil {
		paths = append(paths, path)
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
