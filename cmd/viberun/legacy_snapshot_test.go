// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacySnapshots(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", "internal", "ui", "testdata"))
	origNoColor := os.Getenv("NO_COLOR")
	origTerm := os.Getenv("TERM")
	_ = os.Setenv("NO_COLOR", "1")
	_ = os.Setenv("TERM", "dumb")
	defer func() {
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
	}()

	snapshots := map[string]string{
		"help_global.txt":      renderGlobalHelp(false),
		"help_app.txt":         renderAppHelp(),
		"setup_intro.txt":      renderSetupIntro(&shellState{}),
		"command_help_url.txt": renderCommandHelp("url", scopeGlobal),
	}

	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		for name, content := range snapshots {
			if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
				t.Fatalf("write snapshot %s: %v", name, err)
			}
		}
		return
	}

	for name, want := range snapshots {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("missing snapshot %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("snapshot mismatch for %s", name)
		}
	}
}
