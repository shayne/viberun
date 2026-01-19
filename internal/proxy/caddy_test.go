// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"strings"
	"testing"
)

func TestBuildCaddyConfig(t *testing.T) {
	cfg := Config{
		BaseDomain: "example.com",
		Auth: AuthConfig{
			SigningKey: "test-key",
		},
		Users: []AuthUser{{Username: "primary", Password: "$2a$10$hash"}},
	}
	ports := map[string]int{
		"app":  8080,
		"zeta": 9000,
	}
	cfg.Apps = map[string]AppAccess{
		"app":  {Access: AccessPrivate},
		"zeta": {Access: AccessPublic},
	}
	data, err := BuildCaddyConfig(cfg, ports)
	if err != nil {
		t.Fatalf("BuildCaddyConfig: %v", err)
	}
	if data.ContentType != "text/caddyfile" {
		t.Fatalf("unexpected content type: %q", data.ContentType)
	}
	text := string(data.Body)
	if !strings.Contains(text, "forward_auth "+defaultAuthListenAddr) {
		t.Fatalf("expected forward_auth handler in config")
	}
	if !strings.Contains(text, "uri "+authVerifyPath) {
		t.Fatalf("expected auth verify path in config")
	}
	if !strings.Contains(text, "handle "+authPathPrefix+"/*") {
		t.Fatalf("expected auth handle matcher in config")
	}
	if !strings.Contains(text, "header_up X-Forwarded-Host") {
		t.Fatalf("expected forwarded host header in config")
	}
	if !strings.Contains(text, "app.example.com") {
		t.Fatalf("expected app host")
	}
	if !strings.Contains(text, "reverse_proxy 127.0.0.1:8080") {
		t.Fatalf("expected app reverse proxy")
	}
	if strings.Contains(text, "authorize with") {
		t.Fatalf("did not expect authorize handler in config")
	}
}
