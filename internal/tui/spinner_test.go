// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"bytes"
	"testing"
	"time"
)

func TestSpinnerOptions(t *testing.T) {
	buf := &bytes.Buffer{}
	frames := []string{"*"}
	interval := 10 * time.Millisecond
	code := "\x1b[32m"
	s := NewSpinner(buf, WithFrames(frames), WithInterval(interval), WithHideCursor(true), WithColor(Colorizer{Enabled: true}, code))
	if len(s.frames) != 1 || s.frames[0] != "*" {
		t.Fatalf("unexpected frames: %v", s.frames)
	}
	if s.interval != interval {
		t.Fatalf("unexpected interval: %v", s.interval)
	}
	if !s.hideCursor {
		t.Fatalf("expected hideCursor true")
	}
	if !s.color.Enabled || s.frameColor != code {
		t.Fatalf("unexpected color settings")
	}
}

func TestSpinnerRenderAndClear(t *testing.T) {
	buf := &bytes.Buffer{}
	s := NewSpinner(buf, WithFrames([]string{"."}))
	s.renderFrame(0, "hi")
	if got := buf.String(); got != "\r\033[K. hi" {
		t.Fatalf("unexpected render: %q", got)
	}
	buf.Reset()
	s.clearLine()
	if got := buf.String(); got != "\r\033[K" {
		t.Fatalf("unexpected clear: %q", got)
	}
}
