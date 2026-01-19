// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"strings"
)

func UpsertUser(cfg *Config, username string, password string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	users := cfg.Users
	found := false
	for i := range users {
		if users[i].Username == username {
			users[i].Password = hash
			if strings.TrimSpace(users[i].Email) == "" {
				users[i].Email = username + "@local"
			}
			found = true
			break
		}
	}
	if !found {
		users = append(users, AuthUser{Username: username, Email: username + "@local", Password: hash})
	}
	cfg.Users = normalizeUsers(users)
	return nil
}

func RemoveUser(cfg *Config, username string) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("config is nil")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return false, fmt.Errorf("username is required")
	}
	updated := make([]AuthUser, 0, len(cfg.Users))
	removed := false
	for _, user := range cfg.Users {
		if user.Username == username {
			removed = true
			continue
		}
		updated = append(updated, user)
	}
	cfg.Users = normalizeUsers(updated)
	return removed, nil
}

func Usernames(cfg Config) []string {
	if len(cfg.Users) == 0 {
		return nil
	}
	out := make([]string, 0, len(cfg.Users))
	for _, user := range cfg.Users {
		if user.Username == "" {
			continue
		}
		out = append(out, user.Username)
	}
	return normalizeUserList(out)
}
