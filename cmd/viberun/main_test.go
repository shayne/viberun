// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeArgsBootstrapAlias(t *testing.T) {
	args := []string{"bootstrap", "root@5.161.202.241"}
	want := []string{"setup", "root@5.161.202.241"}
	got := normalizeArgs(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNormalizeArgsHelpSubcommand(t *testing.T) {
	args := []string{"help", "setup"}
	want := []string{"setup", "--help"}
	got := normalizeArgs(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNormalizeArgsHelpUnknown(t *testing.T) {
	args := []string{"help", "config"}
	want := []string{"--help"}
	got := normalizeArgs(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNormalizeArgsVersion(t *testing.T) {
	args := []string{"version"}
	want := []string{"--version"}
	got := normalizeArgs(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":    "''",
		"abc": "'abc'",
		"a'b": "'a'\"'\"'b'",
		" a ": "' a '",
	}
	for input, expected := range cases {
		if got := shellQuote(input); got != expected {
			t.Fatalf("shellQuote(%q)=%q want %q", input, got, expected)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	cases := map[string]bool{
		"localhost":         true,
		"127.0.0.1":         true,
		"::1":               true,
		"user@localhost":    true,
		"user@127.0.0.1:22": true,
		"[::1]:2222":        true,
		"example.com":       false,
		"user@example.com":  false,
		"":                  false,
	}
	for input, expected := range cases {
		if got := isLocalHost(input); got != expected {
			t.Fatalf("isLocalHost(%q)=%v want %v", input, got, expected)
		}
	}
}

func TestValidateOpenURL(t *testing.T) {
	valid := []string{
		"http://localhost:8080",
		"https://example.com/path?x=1",
	}
	for _, raw := range valid {
		if got, err := validateOpenURL(raw); err != nil || got != raw {
			t.Fatalf("validateOpenURL(%q)=%q err=%v", raw, got, err)
		}
	}
	invalid := []string{
		"",
		"   ",
		"ftp://example.com",
		"http://",
		"http://example.com/\npath",
	}
	for _, raw := range invalid {
		if got, err := validateOpenURL(raw); err == nil {
			t.Fatalf("validateOpenURL(%q)=%q expected error", raw, got)
		}
	}
}

func TestNormalizeArch(t *testing.T) {
	cases := map[string]string{
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"arm64":   "arm64",
		"aarch64": "arm64",
	}
	for input, expected := range cases {
		got, err := normalizeArch(input)
		if err != nil || got != expected {
			t.Fatalf("normalizeArch(%q)=%q err=%v want %q", input, got, err, expected)
		}
	}
	if _, err := normalizeArch("mips"); err == nil {
		t.Fatalf("expected error for unsupported arch")
	}
}

func TestReplaceEnv(t *testing.T) {
	env := []string{"A=1", "TERM=xterm"}
	got := replaceEnv(env, "TERM", "xterm-256color")
	if !reflect.DeepEqual(got, []string{"A=1", "TERM=xterm-256color"}) {
		t.Fatalf("replaceEnv update unexpected: %v", got)
	}
	got = replaceEnv(env, "NEW", "val")
	if !reflect.DeepEqual(got, []string{"A=1", "TERM=xterm", "NEW=val"}) {
		t.Fatalf("replaceEnv add unexpected: %v", got)
	}
}

func TestPrintURLSummary(t *testing.T) {
	info := proxyInfo{
		App:          "myapp",
		URL:          "https://myapp.example.com",
		Access:       "private",
		Disabled:     false,
		AllowedUsers: []string{"primary", "secondary"},
		PublicIP:     "1.2.3.4",
	}
	var buf bytes.Buffer
	printURLSummary(&buf, info)
	out := buf.String()
	if !strings.Contains(out, "App: myapp") {
		t.Fatalf("expected app line in output: %s", out)
	}
	if !strings.Contains(out, "Access: requires login") {
		t.Fatalf("expected status line in output: %s", out)
	}
	if !strings.Contains(out, "DNS: create an A record for myapp.example.com -> 1.2.3.4") {
		t.Fatalf("expected dns line in output: %s", out)
	}
	if !strings.Contains(out, "Commands:") {
		t.Fatalf("expected commands header in output: %s", out)
	}
	if !strings.Contains(out, "app myapp url public") {
		t.Fatalf("expected commands list in output: %s", out)
	}
	if !strings.Contains(out, "app myapp url private") {
		t.Fatalf("expected commands list in output: %s", out)
	}
}
