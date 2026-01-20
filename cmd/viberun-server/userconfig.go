// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shayne/viberun/internal/userconfig"
)

const (
	userConfigEnvVar        = "VIBERUN_USER_CONFIG"
	userConfigHostPath      = "/var/lib/viberun/userconfig.json"
	userConfigContainerPath = "/opt/viberun/userconfig.json"
)

func updateUserConfigFromEnv() error {
	raw := strings.TrimSpace(os.Getenv(userConfigEnvVar))
	if raw == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("invalid user config encoding: %w", err)
	}
	var incoming userconfig.Config
	if err := json.Unmarshal(decoded, &incoming); err != nil {
		return fmt.Errorf("invalid user config: %w", err)
	}
	current, err := readUserConfig()
	if err != nil {
		return err
	}
	userconfig.Merge(&current, incoming)
	return writeUserConfig(current)
}

func ensureUserConfigFile() error {
	if err := ensureUserConfigDir(); err != nil {
		return err
	}
	if _, err := os.Stat(userConfigHostPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeUserConfig(userconfig.Config{})
}

func ensureUserConfigDir() error {
	return os.MkdirAll(filepath.Dir(userConfigHostPath), 0o755)
}

func readUserConfig() (userconfig.Config, error) {
	var cfg userconfig.Config
	data, err := os.ReadFile(userConfigHostPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse %s: %w", userConfigHostPath, err)
	}
	return cfg, nil
}

func writeUserConfig(cfg userconfig.Config) error {
	if err := ensureUserConfigDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(userConfigHostPath, data, 0o644)
}
