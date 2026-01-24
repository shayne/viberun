// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelpGlobalParity(t *testing.T) {
	withPlainEnv(t)
	want := readSnapshot(t, "help_global.txt")
	if got := renderHelpGlobal(false); got != want {
		t.Fatalf("help global mismatch")
	}
}

func TestHelpAppParity(t *testing.T) {
	withPlainEnv(t)
	want := readSnapshot(t, "help_app.txt")
	if got := renderHelpApp(); got != want {
		t.Fatalf("help app mismatch")
	}
}

func TestSetupIntroParity(t *testing.T) {
	withPlainEnv(t)
	want := readSnapshot(t, "setup_intro.txt")
	if got := renderSetupIntro(); got != want {
		t.Fatalf("setup intro mismatch")
	}
}

func TestCommandHelpParity(t *testing.T) {
	withPlainEnv(t)
	want := readSnapshot(t, "command_help_url.txt")
	if got := renderCommandHelp("url", scopeGlobal); got != want {
		t.Fatalf("command help mismatch")
	}
}

func readSnapshot(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot %s: %v", name, err)
	}
	return string(data)
}

func withPlainEnv(t *testing.T) {
	t.Helper()
	origNoColor := os.Getenv("NO_COLOR")
	origTerm := os.Getenv("TERM")
	_ = os.Setenv("NO_COLOR", "1")
	_ = os.Setenv("TERM", "dumb")
	t.Cleanup(func() {
		if origNoColor == "" {
			_ = os.Unsetenv("NO_COLOR")
		} else {
			_ = os.Setenv("NO_COLOR", origNoColor)
		}
		if origTerm == "" {
			_ = os.Unsetenv("TERM")
		} else {
			_ = os.Setenv("TERM", origTerm)
		}
	})
}
