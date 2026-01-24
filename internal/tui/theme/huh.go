// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package theme

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func buildHuhTheme(pal tokens) *huh.Theme {
	theme := huh.ThemeBase()
	accent := lipgloss.Color(pal.promptBrand)
	muted := lipgloss.Color(pal.muted)
	label := lipgloss.Color(pal.label)
	value := lipgloss.Color(pal.value)
	header := lipgloss.Color(pal.helpHeader)
	err := lipgloss.Color(pal.error)

	theme.Group.Title = theme.Group.Title.Foreground(header).Bold(true)
	theme.Group.Description = theme.Group.Description.Foreground(muted)
	theme.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")

	theme.Focused.Title = theme.Focused.Title.Foreground(label).Bold(true)
	theme.Focused.Description = theme.Focused.Description.Foreground(muted)
	theme.Focused.ErrorIndicator = theme.Focused.ErrorIndicator.Foreground(err)
	theme.Focused.ErrorMessage = theme.Focused.ErrorMessage.Foreground(err)
	theme.Focused.SelectSelector = theme.Focused.SelectSelector.Foreground(accent)
	theme.Focused.MultiSelectSelector = theme.Focused.MultiSelectSelector.Foreground(accent)
	theme.Focused.TextInput.Prompt = theme.Focused.TextInput.Prompt.Foreground(label)
	theme.Focused.TextInput.Text = theme.Focused.TextInput.Text.Foreground(value)
	theme.Focused.TextInput.Placeholder = theme.Focused.TextInput.Placeholder.Foreground(muted)
	theme.Focused.NextIndicator = theme.Focused.NextIndicator.Foreground(label)
	theme.Focused.PrevIndicator = theme.Focused.PrevIndicator.Foreground(label)

	theme.Blurred = theme.Focused
	theme.Blurred.Base = theme.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	theme.Blurred.Card = theme.Blurred.Base
	return theme
}
