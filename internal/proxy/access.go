// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"sort"
	"strings"
)

const (
	AccessPrivate = "private"
	AccessPublic  = "public"
)

func NormalizeAccessMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccessPublic:
		return AccessPublic
	case "":
		return ""
	default:
		return AccessPrivate
	}
}

func EffectiveAppAccess(cfg Config, app string) AppAccess {
	appCfg, ok := cfg.Apps[app]
	if !ok {
		appCfg.Access = cfg.DefaultAccess
	}
	appCfg.Access = NormalizeAccessMode(appCfg.Access)
	if appCfg.Access == "" {
		appCfg.Access = cfg.DefaultAccess
	}
	appCfg.AllowedUsers = normalizeUserList(appCfg.AllowedUsers)
	appCfg.CustomDomain = strings.TrimSpace(appCfg.CustomDomain)
	return appCfg
}

func EffectiveAllowedUsers(cfg Config, app string) []string {
	primary := strings.TrimSpace(cfg.PrimaryUser)
	access := EffectiveAppAccess(cfg, app)
	users := append([]string{}, access.AllowedUsers...)
	if primary != "" {
		users = append([]string{primary}, users...)
	}
	users = normalizeUserList(users)
	return users
}

func PublicHostForApp(cfg Config, app string) string {
	access := EffectiveAppAccess(cfg, app)
	if access.Disabled {
		return ""
	}
	custom := strings.TrimSpace(access.CustomDomain)
	if custom != "" {
		return strings.ToLower(custom)
	}
	base := strings.TrimSpace(cfg.BaseDomain)
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", app, base)
}

func PublicURLForApp(cfg Config, app string) string {
	host := PublicHostForApp(cfg, app)
	if host == "" {
		return ""
	}
	return "https://" + host
}

func SortedAppNames(apps map[string]AppAccess) []string {
	if len(apps) == 0 {
		return nil
	}
	names := make([]string, 0, len(apps))
	for name := range apps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
