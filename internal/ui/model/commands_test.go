// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import "testing"

func TestSplitShellArgs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{input: "apps", want: []string{"apps"}},
		{input: "app myapp", want: []string{"app", "myapp"}},
		{input: "config set host root@1.2.3.4", want: []string{"config", "set", "host", "root@1.2.3.4"}},
		{input: "config set agent npx:@sourcegraph/amp@latest", want: []string{"config", "set", "agent", "npx:@sourcegraph/amp@latest"}},
		{input: "url set-domain myapp.com", want: []string{"url", "set-domain", "myapp.com"}},
		{input: "app my\\ app", want: []string{"app", "my app"}},
		{input: "config set host 'root@my-host'", want: []string{"config", "set", "host", "root@my-host"}},
		{input: "config set host \"root@my host\"", want: []string{"config", "set", "host", "root@my host"}},
	}
	for _, tc := range cases {
		got, err := splitShellArgs(tc.input)
		if err != nil {
			t.Fatalf("splitShellArgs(%q) returned error: %v", tc.input, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("splitShellArgs(%q)=%v want %v", tc.input, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitShellArgs(%q)=%v want %v", tc.input, got, tc.want)
			}
		}
	}
}

func TestSplitShellArgsUnterminated(t *testing.T) {
	_, err := splitShellArgs("config set host 'root@host")
	if err == nil {
		t.Fatalf("expected error for unterminated quote")
	}
}

func TestParseShellCommandLowercase(t *testing.T) {
	cmd, err := parseShellCommand("ViBe MyApp")
	if err != nil {
		t.Fatalf("parseShellCommand returned error: %v", err)
	}
	if cmd.name != "vibe" {
		t.Fatalf("expected name 'vibe', got %q", cmd.name)
	}
	if len(cmd.args) != 1 || cmd.args[0] != "MyApp" {
		t.Fatalf("unexpected args: %v", cmd.args)
	}
	if cmd.enforceExisting {
		t.Fatalf("expected enforceExisting=false for parsed vibe command")
	}
}

func TestParseShellCommandPreservesUnknownCase(t *testing.T) {
	cmd, err := parseShellCommand("MyApp")
	if err != nil {
		t.Fatalf("parseShellCommand returned error: %v", err)
	}
	if cmd.name != "MyApp" {
		t.Fatalf("expected name 'MyApp', got %q", cmd.name)
	}
	if len(cmd.args) != 0 {
		t.Fatalf("unexpected args: %v", cmd.args)
	}
}
