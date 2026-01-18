// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestCustomAgentParts(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		runner   string
		pkg      string
		ok       bool
	}{
		{
			name:     "npx provider",
			provider: "npx:@sourcegraph/amp@latest",
			runner:   "npx",
			pkg:      "@sourcegraph/amp@latest",
			ok:       true,
		},
		{
			name:     "uvx provider",
			provider: "uvx:llama-index-cli",
			runner:   "uvx",
			pkg:      "llama-index-cli",
			ok:       true,
		},
		{
			name:     "case insensitive prefix",
			provider: "NPX:@OpenAI/Codex@latest",
			runner:   "npx",
			pkg:      "@OpenAI/Codex@latest",
			ok:       true,
		},
		{
			name:     "builtin provider",
			provider: "codex",
			ok:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner, pkg, ok := customAgentParts(test.provider)
			if ok != test.ok {
				t.Fatalf("customAgentParts(%q) ok=%v want %v", test.provider, ok, test.ok)
			}
			if runner != test.runner {
				t.Fatalf("customAgentParts(%q) runner=%q want %q", test.provider, runner, test.runner)
			}
			if pkg != test.pkg {
				t.Fatalf("customAgentParts(%q) pkg=%q want %q", test.provider, pkg, test.pkg)
			}
		})
	}
}

func TestLooksLikeCustomAgentFailure(t *testing.T) {
	ok := looksLikeCustomAgentFailure("custom agent \"npx:bad\" failed to start inside the container (exit code 1).")
	if !ok {
		t.Fatalf("expected custom agent failure to be detected")
	}
	notOk := looksLikeCustomAgentFailure("some other error")
	if notOk {
		t.Fatalf("unexpected custom agent failure detection")
	}
}
