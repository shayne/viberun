// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package styles

import "charm.land/lipgloss/v2"

// Styles defines the minimal semantic style set used by the v2 UI.
type Styles struct {
	Header lipgloss.Style
	Muted  lipgloss.Style
	Value  lipgloss.Style
	Link   lipgloss.Style
	Error  lipgloss.Style

	HelpLabel    lipgloss.Style
	HelpUsage    lipgloss.Style
	HelpExamples lipgloss.Style
}

// DefaultStyles returns the base semantic styles for shell rendering.
func DefaultStyles() Styles {
	return Styles{
		Header:       lipgloss.NewStyle().Bold(true),
		Muted:        lipgloss.NewStyle().Faint(true),
		Value:        lipgloss.NewStyle(),
		Link:         lipgloss.NewStyle().Underline(true),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		HelpLabel:    lipgloss.NewStyle().Bold(true),
		HelpUsage:    lipgloss.NewStyle().Bold(true),
		HelpExamples: lipgloss.NewStyle().Bold(true),
	}
}
