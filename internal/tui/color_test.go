// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import "testing"

func TestNewColorizerDisabled(t *testing.T) {
	if got := NewColorizer(false); got.Enabled {
		t.Fatalf("expected disabled colorizer")
	}
}

func TestNewColorizerWithNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	if got := NewColorizer(true); got.Enabled {
		t.Fatalf("expected disabled colorizer when NO_COLOR is set")
	}
}

func TestNewColorizerWithDumbTerm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	if got := NewColorizer(true); got.Enabled {
		t.Fatalf("expected disabled colorizer for dumb term")
	}
}

func TestColorizerWrap(t *testing.T) {
	c := Colorizer{Enabled: true}
	got := c.Wrap(ColorRed, "hi")
	if got != ColorRed+"hi"+ColorReset {
		t.Fatalf("unexpected wrap: %q", got)
	}
	c = Colorizer{}
	if got := c.Wrap(ColorRed, "hi"); got != "hi" {
		t.Fatalf("expected no color wrap, got %q", got)
	}
}
