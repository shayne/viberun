// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package theme

import "testing"

func TestParseOSCResponseBEL(t *testing.T) {
	resp := "\x1b]11;rgb:ff/00/80\a"
	got, ok := parseOSCResponse(resp, 11)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if got.R != 255 || got.G != 0 || got.B != 128 {
		t.Fatalf("unexpected rgb: %+v", got)
	}
}

func TestParseOSCResponseST(t *testing.T) {
	resp := "\x1b]10;rgb:ffff/0000/7fff\x1b\\"
	got, ok := parseOSCResponse(resp, 10)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if got.R != 255 || got.G != 0 || got.B == 0 {
		t.Fatalf("unexpected rgb: %+v", got)
	}
}

func TestModeFromPalette(t *testing.T) {
	light := Palette{BG: RGB{R: 250, G: 250, B: 250}, HasBG: true}
	if got := modeFromPalette(light); got != ModeLight {
		t.Fatalf("expected light mode, got %v", got)
	}
	dark := Palette{BG: RGB{R: 10, G: 10, B: 10}, HasBG: true}
	if got := modeFromPalette(dark); got != ModeDark {
		t.Fatalf("expected dark mode, got %v", got)
	}
	unknown := Palette{}
	if got := modeFromPalette(unknown); got != ModeUnknown {
		t.Fatalf("expected unknown mode, got %v", got)
	}
}
