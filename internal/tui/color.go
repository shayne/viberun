// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"io"

	"github.com/shayne/viberun/internal/tui/theme"
)

type Colorizer struct {
	Enabled bool
	ansi    theme.AnsiStyles
}

func NewColorizer(out io.Writer, enabled bool) Colorizer {
	if !enabled {
		return Colorizer{}
	}
	selected := theme.ForOutput(out)
	if !selected.Enabled {
		return Colorizer{}
	}
	return Colorizer{Enabled: true, ansi: selected.ANSI}
}

func (c Colorizer) Wrap(code, text string) string {
	if !c.Enabled || code == "" {
		return text
	}
	reset := c.ansi.Reset
	if reset == "" {
		reset = "\x1b[0m"
	}
	return code + text + reset
}
