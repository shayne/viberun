// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const caddyfileContentType = "text/caddyfile"
const authPathPrefix = "/__viberun/auth"
const authVerifyPath = "/__viberun/auth/verify"

// CaddyConfig holds the generated config and content type.
type CaddyConfig struct {
	Body        []byte
	ContentType string
}

func BuildCaddyConfig(cfg Config, ports map[string]int) (CaddyConfig, error) {
	cfg = applyDefaults(cfg)
	if strings.TrimSpace(cfg.BaseDomain) == "" {
		return CaddyConfig{}, fmt.Errorf("base domain is required")
	}
	if strings.TrimSpace(cfg.Auth.SigningKey) == "" {
		return CaddyConfig{}, fmt.Errorf("auth signing key is required")
	}
	if len(cfg.Users) == 0 {
		return CaddyConfig{}, fmt.Errorf("at least one user is required")
	}

	authAddr := strings.TrimSpace(cfg.Auth.ListenAddr)
	if authAddr == "" {
		authAddr = defaultAuthListenAddr
	}

	var b bytes.Buffer
	b.WriteString("{\n")
	b.WriteString("  admin " + cfg.AdminAddr + "\n")
	appNames := make([]string, 0, len(ports))
	for app := range ports {
		appNames = append(appNames, app)
	}
	sort.Strings(appNames)
	b.WriteString("}\n\n")

	for _, app := range appNames {
		port := ports[app]
		if port <= 0 {
			continue
		}
		access := EffectiveAppAccess(cfg, app)
		if access.Disabled {
			continue
		}
		host := PublicHostForApp(cfg, app)
		if host == "" {
			continue
		}
		b.WriteString(host + " {\n")
		b.WriteString("  handle " + authPathPrefix + "/* {\n")
		b.WriteString("    reverse_proxy " + authAddr + "\n")
		b.WriteString("  }\n")
		if access.Access == AccessPrivate {
			b.WriteString("  handle {\n")
			b.WriteString("    forward_auth " + authAddr + " {\n")
			b.WriteString("      uri " + authVerifyPath + "\n")
			b.WriteString("      copy_headers X-Viberun-User X-Viberun-Roles\n")
			b.WriteString("      header_up X-Forwarded-Host {host}\n")
			b.WriteString("      header_up X-Forwarded-Proto {scheme}\n")
			b.WriteString("    }\n")
			b.WriteString("    reverse_proxy 127.0.0.1:" + strconv.Itoa(port) + "\n")
			b.WriteString("  }\n")
		} else {
			b.WriteString("  handle {\n")
			b.WriteString("    reverse_proxy 127.0.0.1:" + strconv.Itoa(port) + "\n")
			b.WriteString("  }\n")
		}
		b.WriteString("}\n\n")
	}

	return CaddyConfig{Body: b.Bytes(), ContentType: caddyfileContentType}, nil
}
