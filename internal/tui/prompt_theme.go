// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"io"

	"github.com/charmbracelet/huh"
	"github.com/shayne/viberun/internal/tui/theme"
)

func promptTheme(out io.Writer) *huh.Theme {
	selected := theme.ForOutput(out)
	if selected.Enabled && selected.Huh != nil {
		return selected.Huh
	}
	return huh.ThemeCharm()
}

func PromptTheme(out io.Writer) *huh.Theme {
	return promptTheme(out)
}
