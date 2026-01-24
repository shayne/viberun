// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/theme"
)

func TestNewColorizerDisabled(t *testing.T) {
	if got := NewColorizer(testTTY{}, false); got.Enabled {
		t.Fatalf("expected disabled colorizer")
	}
}

func TestNewColorizerWithNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	if got := NewColorizer(testTTY{}, true); got.Enabled {
		t.Fatalf("expected disabled colorizer when NO_COLOR is set")
	}
}

func TestNewColorizerWithDumbTerm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	if got := NewColorizer(testTTY{}, true); got.Enabled {
		t.Fatalf("expected disabled colorizer for dumb term")
	}
}

func TestColorizerWrap(t *testing.T) {
	c := Colorizer{Enabled: true, ansi: theme.AnsiStyles{Reset: "\x1b[0m"}}
	code := "\x1b[31m"
	got := c.Wrap(code, "hi")
	if got != code+"hi"+"\x1b[0m" {
		t.Fatalf("unexpected wrap: %q", got)
	}
	c = Colorizer{}
	if got := c.Wrap(code, "hi"); got != "hi" {
		t.Fatalf("expected no color wrap, got %q", got)
	}
}

type testTTY struct{}

func (testTTY) Write(p []byte) (int, error) { return len(p), nil }
func (testTTY) IsTTY() bool                 { return true }
