// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shayne/viberun/internal/tui/theme"
)

type ConfirmDialog struct {
	id          string
	title       string
	description string
	defaultYes  bool
	result      *Result
	input       textinput.Model
	err         string
}

func NewConfirmDialog(id, title, description string, defaultYes bool) *ConfirmDialog {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 3
	ti.SetVirtualCursor(true)
	ti.Focus()
	return &ConfirmDialog{id: id, title: title, description: description, defaultYes: defaultYes, input: ti}
}

func (d *ConfirmDialog) ID() string { return d.id }
func (d *ConfirmDialog) Init() tea.Cmd {
	return d.input.Focus()
}

func (d *ConfirmDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			value := strings.ToLower(strings.TrimSpace(d.input.Value()))
			if value == "" {
				d.result = &Result{Confirmed: d.defaultYes}
				d.err = ""
				return d, nil
			}
			if confirmed, ok := parseConfirmInput(value); ok {
				d.result = &Result{Confirmed: confirmed}
				d.err = ""
				return d, nil
			}
			d.err = "enter y or n"
			return d, nil
		case "esc", "ctrl+c":
			d.result = &Result{Cancelled: true}
			return d, nil
		}
	}
	d.err = ""
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d *ConfirmDialog) View() string {
	t := theme.ForOutput(os.Stdout)
	headerStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle()
	promptStyle := lipgloss.NewStyle()
	errStyle := lipgloss.NewStyle()
	if t.Enabled {
		headerStyle = t.Shell.HelpHeader
		descStyle = t.Shell.Muted
		errStyle = t.Shell.Error
	}
	lines := []string{}
	if d.title != "" {
		lines = append(lines, headerStyle.Render(d.title))
	}
	if d.description != "" {
		lines = append(lines, descStyle.Render(d.description))
	}
	prompt := "Confirm [y/N]"
	if d.defaultYes {
		prompt = "Confirm [Y/n]"
	}
	lines = append(lines, promptStyle.Render(prompt)+" "+d.input.View())
	if d.err != "" {
		lines = append(lines, errStyle.Render(d.err))
	}
	return strings.Join(lines, "\n")
}

func (d *ConfirmDialog) Cursor() *tea.Cursor { return nil }

func (d *ConfirmDialog) Result() (*Result, bool) {
	if d.result == nil {
		return nil, false
	}
	return d.result, true
}

func (d *ConfirmDialog) SubmitDefault() {
	d.result = &Result{Confirmed: d.defaultYes}
}

func parseConfirmInput(value string) (bool, bool) {
	switch value {
	case "y", "yes":
		return true, true
	case "n", "no":
		return false, true
	default:
		return false, false
	}
}
