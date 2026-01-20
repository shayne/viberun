// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/shayne/viberun/internal/userconfig"
)

func discoverLocalUserConfig() (*userconfig.Config, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	name, err := gitConfigValue("user.name")
	if err != nil {
		return nil, err
	}
	email, err := gitConfigValue("user.email")
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" && email == "" {
		return nil, nil
	}
	cfg := &userconfig.Config{
		Git: &userconfig.GitConfig{
			Name:  name,
			Email: email,
		},
	}
	return cfg, nil
}

func gitConfigValue(key string) (string, error) {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func encodeUserConfig(cfg *userconfig.Config) (string, error) {
	if cfg == nil {
		return "", nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
