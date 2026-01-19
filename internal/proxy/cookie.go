// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import "strings"

// DefaultCookieDomain returns the base domain to use for auth cookies.
// If custom domains are outside the base domain, an empty string is returned.
func DefaultCookieDomain(cfg Config) string {
	base := strings.TrimPrefix(strings.TrimSpace(cfg.BaseDomain), ".")
	if base == "" {
		return ""
	}
	for app := range cfg.Apps {
		custom := strings.TrimSpace(cfg.Apps[app].CustomDomain)
		if custom == "" {
			continue
		}
		custom = strings.ToLower(custom)
		if custom == base || strings.HasSuffix(custom, "."+base) {
			continue
		}
		return ""
	}
	return "." + base
}
