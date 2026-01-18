// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestSanitizeHostRPCName(t *testing.T) {
	cases := map[string]string{
		"":             "app",
		" MyApp ":      "myapp",
		"hello/world":  "hello_world",
		"foo.bar":      "foo.bar",
		"a b c":        "a_b_c",
		"UPPER_CASE":   "upper_case",
		"weird!chars":  "weird_chars",
		"already-good": "already-good",
	}
	for input, expected := range cases {
		if got := sanitizeHostRPCName(input); got != expected {
			t.Fatalf("sanitizeHostRPCName(%q)=%q want %q", input, got, expected)
		}
	}
}

func TestHostRPCConfigForAppBase(t *testing.T) {
	cfg := hostRPCConfigForAppBase("myapp", "/tmp/base")
	if cfg.HostDir != "/tmp/base/myapp" {
		t.Fatalf("unexpected host dir: %q", cfg.HostDir)
	}
	if cfg.ContainerDir == "" || cfg.ContainerSocket == "" || cfg.ContainerTokenFile == "" {
		t.Fatalf("expected container paths, got %+v", cfg)
	}
}
