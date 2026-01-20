// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package userconfig

// Config is host-provided configuration applied inside containers.
type Config struct {
	Git *GitConfig `json:"git,omitempty"`
}

// GitConfig holds Git identity defaults.
type GitConfig struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Merge applies non-empty values from src onto dst.
func Merge(dst *Config, src Config) {
	if dst == nil {
		return
	}
	if src.Git != nil {
		if dst.Git == nil {
			dst.Git = &GitConfig{}
		}
		if src.Git.Name != "" {
			dst.Git.Name = src.Git.Name
		}
		if src.Git.Email != "" {
			dst.Git.Email = src.Git.Email
		}
	}
}
