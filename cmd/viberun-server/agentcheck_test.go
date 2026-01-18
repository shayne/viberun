// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestCustomAgentRunner(t *testing.T) {
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
			runner, pkg, ok := customAgentRunner(test.provider)
			if ok != test.ok {
				t.Fatalf("customAgentRunner(%q) ok=%v want %v", test.provider, ok, test.ok)
			}
			if runner != test.runner {
				t.Fatalf("customAgentRunner(%q) runner=%q want %q", test.provider, runner, test.runner)
			}
			if pkg != test.pkg {
				t.Fatalf("customAgentRunner(%q) pkg=%q want %q", test.provider, pkg, test.pkg)
			}
		})
	}
}

func TestCustomAgentCheckArgs(t *testing.T) {
	if args := customAgentCheckArgs("npx", "my-agent"); len(args) != 4 || args[0] != "npx" || args[1] != "-y" || args[2] != "my-agent" || args[3] != "--help" {
		t.Fatalf("unexpected npx args: %v", args)
	}
	if args := customAgentCheckArgs("uvx", "my-agent"); len(args) != 3 || args[0] != "uvx" || args[1] != "my-agent" || args[2] != "--help" {
		t.Fatalf("unexpected uvx args: %v", args)
	}
	if args := customAgentCheckArgs("unknown", "my-agent"); args != nil {
		t.Fatalf("unexpected args for unknown runner: %v", args)
	}
}
