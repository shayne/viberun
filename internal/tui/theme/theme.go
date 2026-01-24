// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package theme

import (
	"io"
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

type Mode int

const (
	ModeUnknown Mode = iota
	ModeLight
	ModeDark
)

type RGB struct {
	R uint8
	G uint8
	B uint8
}

type Palette struct {
	FG      RGB
	BG      RGB
	HasFG   bool
	HasBG   bool
	Version uint64
}

type Theme struct {
	Enabled bool
	Mode    Mode
	Palette Palette
	Shell   ShellStyles
	URL     URLStyles
	ANSI    AnsiStyles
}

type ShellStyles struct {
	Brand             lipgloss.Style
	PromptBrand       lipgloss.Style
	PromptArrow       lipgloss.Style
	Header            lipgloss.Style
	Label             lipgloss.Style
	Value             lipgloss.Style
	Muted             lipgloss.Style
	Link              lipgloss.Style
	Error             lipgloss.Style
	HelpHeader        lipgloss.Style
	HelpLabel         lipgloss.Style
	HelpUsage         lipgloss.Style
	HelpExamples      lipgloss.Style
	BannerVerb        lipgloss.Style
	BannerApp         lipgloss.Style
	BannerHelp        lipgloss.Style
	StatusConnected   lipgloss.Style
	StatusConnecting  lipgloss.Style
	StatusFailed      lipgloss.Style
	StatusRunning     lipgloss.Style
	StatusStopped     lipgloss.Style
	StatusUnknown     lipgloss.Style
	StatusUnavailable lipgloss.Style
}

type URLStyles struct {
	Label    lipgloss.Style
	Value    lipgloss.Style
	Header   lipgloss.Style
	Command  lipgloss.Style
	Comment  lipgloss.Style
	Link     lipgloss.Style
	Public   lipgloss.Style
	Private  lipgloss.Style
	Disabled lipgloss.Style
	IP       lipgloss.Style
}

type AnsiStyles struct {
	Reset   string
	Success string
	Error   string
	Warning string
	Spinner string
	Dim     string
}

type manager struct {
	mu           sync.Mutex
	palette      Palette
	attempted    bool
	cachedTheme  Theme
	themeVersion uint64
}

var global = &manager{}

func ForShell() Theme {
	return ForOutput(os.Stdout)
}

func ForOutput(out io.Writer) Theme {
	enabled := EnabledForOutput(out)
	return global.themeFor(enabled)
}

func Refresh() {
	global.refresh()
}

func EnabledForOutput(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	termValue := os.Getenv("TERM")
	if termValue == "" || termValue == "dumb" {
		return false
	}
	if ttyAware, ok := out.(interface{ IsTTY() bool }); ok {
		return ttyAware.IsTTY()
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (m *manager) themeFor(enabled bool) Theme {
	if !enabled {
		return Theme{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePaletteLocked()
	if m.themeVersion != m.palette.Version || !m.cachedTheme.Enabled {
		m.cachedTheme = buildTheme(m.palette)
		m.themeVersion = m.palette.Version
	}
	return m.cachedTheme
}

func (m *manager) refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshLocked()
}

func (m *manager) ensurePaletteLocked() {
	if m.attempted {
		return
	}
	m.attempted = true
	m.refreshLocked()
}

func (m *manager) refreshLocked() {
	palette := queryPalette()
	if !(palette.HasBG || palette.HasFG) {
		return
	}
	if paletteEqual(palette, m.palette) {
		return
	}
	palette.Version = m.palette.Version + 1
	m.palette = palette
}

func paletteEqual(a, b Palette) bool {
	if a.HasFG != b.HasFG || a.HasBG != b.HasBG {
		return false
	}
	if a.HasFG && a.FG != b.FG {
		return false
	}
	if a.HasBG && a.BG != b.BG {
		return false
	}
	return true
}

type tokens struct {
	promptBrand      string
	muted            string
	label            string
	value            string
	link             string
	helpHeader       string
	error            string
	statusConnected  string
	statusConnecting string
	statusFailed     string
	statusUnknown    string
	bannerApp        string
	urlPublic        string
	urlPrivate       string
	urlDisabled      string
	ip               string
}

var darkTokens = tokens{
	promptBrand:      "213",
	muted:            "243",
	label:            "244",
	value:            "252",
	link:             "#7AB8FF",
	helpHeader:       "81",
	error:            "203",
	statusConnected:  "77",
	statusConnecting: "226",
	statusFailed:     "203",
	statusUnknown:    "214",
	bannerApp:        "252",
	urlPublic:        "#7EE787",
	urlPrivate:       "#F2C14E",
	urlDisabled:      "#9CA3AF",
	ip:               "#7EE787",
}

var lightTokens = tokens{
	promptBrand:      "213",
	muted:            "240",
	label:            "238",
	value:            "234",
	link:             "#0A3E84",
	helpHeader:       "23",
	error:            "160",
	statusConnected:  "28",
	statusConnecting: "94",
	statusFailed:     "160",
	statusUnknown:    "94",
	bannerApp:        "234",
	urlPublic:        "#155C15",
	urlPrivate:       "#7A5200",
	urlDisabled:      "#3D3D3D",
	ip:               "#155C15",
}

func buildTheme(palette Palette) Theme {
	mode := modeFromPalette(palette)
	if mode == ModeUnknown {
		mode = ModeDark
	}

	pal := darkTokens
	if mode == ModeLight {
		pal = lightTokens
	}

	shell := ShellStyles{
		Brand:             lipgloss.NewStyle().Bold(true),
		PromptBrand:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.promptBrand)),
		PromptArrow:       lipgloss.NewStyle().Foreground(lipgloss.Color(pal.muted)),
		Header:            lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.helpHeader)),
		Label:             lipgloss.NewStyle().Foreground(lipgloss.Color(pal.label)),
		Value:             lipgloss.NewStyle().Foreground(lipgloss.Color(pal.value)),
		Muted:             lipgloss.NewStyle().Foreground(lipgloss.Color(pal.muted)),
		Link:              lipgloss.NewStyle().Foreground(lipgloss.Color(pal.link)).Underline(true),
		Error:             lipgloss.NewStyle().Foreground(lipgloss.Color(pal.error)),
		HelpHeader:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.helpHeader)),
		HelpLabel:         lipgloss.NewStyle().Bold(true),
		HelpUsage:         lipgloss.NewStyle().Bold(true),
		HelpExamples:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.helpHeader)),
		BannerVerb:        lipgloss.NewStyle().Bold(true),
		BannerApp:         lipgloss.NewStyle().Foreground(lipgloss.Color(pal.bannerApp)).Italic(true),
		BannerHelp:        lipgloss.NewStyle().Bold(true),
		StatusConnected:   lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusConnected)),
		StatusConnecting:  lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusConnecting)),
		StatusFailed:      lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusFailed)),
		StatusRunning:     lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusConnected)),
		StatusStopped:     lipgloss.NewStyle().Foreground(lipgloss.Color(pal.muted)),
		StatusUnknown:     lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusUnknown)),
		StatusUnavailable: lipgloss.NewStyle().Foreground(lipgloss.Color(pal.statusFailed)),
	}

	urlStyles := URLStyles{
		Label:    shell.Muted,
		Value:    shell.Value,
		Header:   shell.HelpHeader,
		Command:  lipgloss.NewStyle(),
		Comment:  shell.Muted,
		Link:     shell.Link,
		Public:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.urlPublic)),
		Private:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.urlPrivate)),
		Disabled: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(pal.urlDisabled)),
		IP:       lipgloss.NewStyle().Foreground(lipgloss.Color(pal.ip)),
	}

	ansi := AnsiStyles{
		Reset:   "\x1b[0m",
		Success: "\x1b[32m",
		Error:   "\x1b[31m",
		Warning: "\x1b[33m",
		Spinner: "\x1b[33m",
		Dim:     "\x1b[90m",
	}

	return Theme{
		Enabled: true,
		Mode:    mode,
		Palette: palette,
		Shell:   shell,
		URL:     urlStyles,
		ANSI:    ansi,
	}
}
