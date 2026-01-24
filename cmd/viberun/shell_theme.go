// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/shayne/viberun/internal/tui/theme"
)

type ShellTheme struct {
	Enabled bool
	theme.ShellStyles
	Palette theme.Palette
	Mode    theme.Mode
}

type themeRefreshMsg struct{}

func refreshShellThemeCmd() tea.Cmd {
	return func() tea.Msg {
		theme.Refresh()
		return themeRefreshMsg{}
	}
}

func shellTheme() ShellTheme {
	selected := theme.ForShell()
	if !selected.Enabled {
		return ShellTheme{}
	}
	return ShellTheme{
		Enabled:     true,
		ShellStyles: selected.Shell,
		Palette:     selected.Palette,
		Mode:        selected.Mode,
	}
}
