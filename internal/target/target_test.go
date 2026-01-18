// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package target

import (
	"testing"

	"github.com/shayne/viberun/internal/config"
)

func TestResolveUsesDefaultHost(t *testing.T) {
	cfg := config.Config{DefaultHost: "host-a"}
	resolved, err := Resolve("myapp", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.App != "myapp" {
		t.Fatalf("expected app myapp, got %q", resolved.App)
	}
	if resolved.Host != "host-a" {
		t.Fatalf("expected host host-a, got %q", resolved.Host)
	}
	if resolved.HostAlias != "" {
		t.Fatalf("expected empty alias, got %q", resolved.HostAlias)
	}
}

func TestResolveUsesExplicitHost(t *testing.T) {
	cfg := config.Config{DefaultHost: "host-a"}
	resolved, err := Resolve("myapp@host-b", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.App != "myapp" {
		t.Fatalf("expected app myapp, got %q", resolved.App)
	}
	if resolved.Host != "host-b" {
		t.Fatalf("expected host host-b, got %q", resolved.Host)
	}
}

func TestResolveAliasLookup(t *testing.T) {
	cfg := config.Config{
		DefaultHost: "host-a",
		Hosts: map[string]string{
			"prod": "ssh://prod.example.com",
		},
	}
	resolved, err := Resolve("myapp@prod", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.Host != "ssh://prod.example.com" {
		t.Fatalf("expected resolved host, got %q", resolved.Host)
	}
	if resolved.HostAlias != "prod" {
		t.Fatalf("expected alias prod, got %q", resolved.HostAlias)
	}
}

func TestResolveMissingHost(t *testing.T) {
	cfg := config.Config{}
	_, err := Resolve("myapp", cfg)
	if err == nil {
		t.Fatalf("expected error for missing default host")
	}
}

func TestResolveRejectsInvalidTargets(t *testing.T) {
	cases := []string{"@", "app@", "@host", "app@host@extra"}
	cfg := config.Config{DefaultHost: "host-a"}
	for _, input := range cases {
		if _, err := Resolve(input, cfg); err == nil {
			t.Fatalf("expected error for input %q", input)
		}
	}
}

func TestResolveHostUsesDefault(t *testing.T) {
	cfg := config.Config{DefaultHost: "host-a"}
	resolved, err := ResolveHost("", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.Host != "host-a" {
		t.Fatalf("expected host host-a, got %q", resolved.Host)
	}
	if resolved.HostAlias != "" {
		t.Fatalf("expected empty alias, got %q", resolved.HostAlias)
	}
}

func TestResolveHostUsesExplicit(t *testing.T) {
	cfg := config.Config{DefaultHost: "host-a"}
	resolved, err := ResolveHost("host-b", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.Host != "host-b" {
		t.Fatalf("expected host host-b, got %q", resolved.Host)
	}
}

func TestResolveHostResolvesAlias(t *testing.T) {
	cfg := config.Config{
		DefaultHost: "host-a",
		Hosts: map[string]string{
			"prod": "ssh://prod.example.com",
		},
	}
	resolved, err := ResolveHost("prod", cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolved.Host != "ssh://prod.example.com" {
		t.Fatalf("expected resolved host, got %q", resolved.Host)
	}
	if resolved.HostAlias != "prod" {
		t.Fatalf("expected alias prod, got %q", resolved.HostAlias)
	}
}

func TestResolveHostMissingDefault(t *testing.T) {
	cfg := config.Config{}
	if _, err := ResolveHost("", cfg); err == nil {
		t.Fatalf("expected error for missing default host")
	}
}
