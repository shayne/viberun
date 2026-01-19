// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import "testing"

func TestEffectiveAllowedUsersIncludesPrimary(t *testing.T) {
	cfg := Config{PrimaryUser: "primary", DefaultAccess: AccessPrivate}
	cfg.Apps = map[string]AppAccess{
		"app": {AllowedUsers: []string{"secondary", "secondary", ""}},
	}
	users := EffectiveAllowedUsers(cfg, "app")
	if len(users) != 2 || users[0] != "primary" || users[1] != "secondary" {
		t.Fatalf("unexpected users: %#v", users)
	}
}

func TestPublicHostForApp(t *testing.T) {
	cfg := Config{BaseDomain: "example.com", DefaultAccess: AccessPrivate}
	if got := PublicHostForApp(cfg, "demo"); got != "demo.example.com" {
		t.Fatalf("expected demo.example.com, got %q", got)
	}
	cfg.Apps = map[string]AppAccess{"demo": {CustomDomain: "MyName.com"}}
	if got := PublicHostForApp(cfg, "demo"); got != "myname.com" {
		t.Fatalf("expected custom domain, got %q", got)
	}
	cfg.Apps["demo"] = AppAccess{Disabled: true}
	if got := PublicHostForApp(cfg, "demo"); got != "" {
		t.Fatalf("expected no host when disabled, got %q", got)
	}
}
