// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
)

type ShellTheme struct {
	Enabled          bool
	Brand            lipgloss.Style
	PromptBrand      lipgloss.Style
	PromptArrow      lipgloss.Style
	Header           lipgloss.Style
	Label            lipgloss.Style
	Value            lipgloss.Style
	Muted            lipgloss.Style
	Link             lipgloss.Style
	Error            lipgloss.Style
	HelpHeader       lipgloss.Style
	HelpLabel        lipgloss.Style
	HelpUsage        lipgloss.Style
	HelpExamples     lipgloss.Style
	BannerVerb       lipgloss.Style
	BannerApp        lipgloss.Style
	BannerHelp       lipgloss.Style
	StatusConnected  lipgloss.Style
	StatusConnecting lipgloss.Style
	StatusFailed     lipgloss.Style
	StatusRunning    lipgloss.Style
	StatusStopped    lipgloss.Style
	StatusUnknown    lipgloss.Style
	StatusUnavailable lipgloss.Style
}

func shellTheme() ShellTheme {
	return newShellTheme(shellStylesEnabled())
}

func shellThemeForOutput(out io.Writer) ShellTheme {
	return newShellTheme(wantPrettyOutput(out))
}

func newShellTheme(enabled bool) ShellTheme {
	if !enabled {
		return ShellTheme{}
	}
	brand := lipgloss.NewStyle().Bold(true)
	promptBrand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	value := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	link := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0A66C2", Dark: "#7AB8FF"}).Underline(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	return ShellTheme{
		Enabled:          true,
		Brand:            brand,
		PromptBrand:      promptBrand,
		PromptArrow:      muted,
		Header:           helpHeader,
		Label:            label,
		Value:            value,
		Muted:            muted,
		Link:             link,
		Error:            errStyle,
		HelpHeader:       helpHeader,
		HelpLabel:        lipgloss.NewStyle().Bold(true),
		HelpUsage:        lipgloss.NewStyle().Bold(true),
		HelpExamples:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		BannerVerb:       lipgloss.NewStyle().Bold(true),
		BannerApp:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Italic(true),
		BannerHelp:       lipgloss.NewStyle().Bold(true),
		StatusConnected:  lipgloss.NewStyle().Foreground(lipgloss.Color("77")),
		StatusConnecting: lipgloss.NewStyle().Foreground(lipgloss.Color("226")),
		StatusFailed:     lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		StatusRunning:    lipgloss.NewStyle().Foreground(lipgloss.Color("77")),
		StatusStopped:    muted,
		StatusUnknown:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		StatusUnavailable: lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
	}
}

func shellStylesEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	termValue := os.Getenv("TERM")
	if termValue == "" || termValue == "dumb" {
		return false
	}
	return true
}
