// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package target

import (
	"fmt"
	"strings"

	"github.com/shayne/viberun/internal/config"
)

type Resolved struct {
	App       string
	Host      string
	HostAlias string
}

type ResolvedHost struct {
	Host      string
	HostAlias string
}

func Resolve(raw string, cfg config.Config) (Resolved, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Resolved{}, fmt.Errorf("app name is required")
	}

	app, host, err := splitTarget(raw)
	if err != nil {
		return Resolved{}, err
	}

	alias := ""
	if host == "" {
		host = strings.TrimSpace(cfg.DefaultHost)
		if host == "" {
			return Resolved{}, fmt.Errorf("no host provided and no default host configured")
		}
	}

	alias, host = resolveHostAlias(host, cfg)

	return Resolved{App: app, Host: host, HostAlias: alias}, nil
}

func ResolveHost(raw string, cfg config.Config) (ResolvedHost, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		host = strings.TrimSpace(cfg.DefaultHost)
		if host == "" {
			return ResolvedHost{}, fmt.Errorf("no host provided and no default host configured")
		}
	}

	alias, host := resolveHostAlias(host, cfg)
	return ResolvedHost{Host: host, HostAlias: alias}, nil
}

func splitTarget(raw string) (string, string, error) {
	if strings.Count(raw, "@") == 0 {
		return raw, "", nil
	}
	if strings.Count(raw, "@") > 1 {
		return "", "", fmt.Errorf("invalid target: too many '@' characters")
	}

	idx := strings.Index(raw, "@")
	app := strings.TrimSpace(raw[:idx])
	host := strings.TrimSpace(raw[idx+1:])
	if app == "" || host == "" {
		return "", "", fmt.Errorf("invalid target: expected app@host")
	}

	return app, host, nil
}

func resolveHostAlias(host string, cfg config.Config) (string, string) {
	if cfg.Hosts != nil {
		if resolved, ok := cfg.Hosts[host]; ok {
			return host, resolved
		}
	}
	return "", host
}
