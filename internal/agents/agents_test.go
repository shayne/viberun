// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package agents

import "testing"

func TestResolveDefault(t *testing.T) {
	spec, err := Resolve("")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if spec.Provider != "codex" {
		t.Fatalf("expected codex provider, got %q", spec.Provider)
	}
	if len(spec.Command) == 0 {
		t.Fatalf("expected command for codex")
	}
}

func TestResolveAlias(t *testing.T) {
	spec, err := Resolve("claude-code")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if spec.Provider != "claude" {
		t.Fatalf("expected claude provider, got %q", spec.Provider)
	}
}

func TestResolveNpxAgent(t *testing.T) {
	spec, err := Resolve("npx:@sourcegraph/amp@latest")
	if err != nil {
		t.Fatalf("resolve npx: %v", err)
	}
	if spec.Provider != "npx:@sourcegraph/amp@latest" {
		t.Fatalf("unexpected provider: %q", spec.Provider)
	}
	if len(spec.Command) != 3 || spec.Command[0] != "npx" || spec.Command[1] != "-y" {
		t.Fatalf("unexpected command: %v", spec.Command)
	}
	if spec.Label != "amp" {
		t.Fatalf("unexpected label: %q", spec.Label)
	}
}

func TestResolveUvxMissingPackage(t *testing.T) {
	if _, err := Resolve("uvx:"); err == nil {
		t.Fatalf("expected error for missing uvx package")
	}
}
