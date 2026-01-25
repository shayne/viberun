// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"cmp"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shayne/viberun/internal/tui/theme"
)

type Field struct {
	ID          string
	Title       string
	Description string
	Placeholder string
	Required    bool
	Secret      bool
	Default     string
	Validate    func(string) error
}

type FormDialog struct {
	id          string
	title       string
	description string
	fields      []Field
	inputs      []textinput.Model
	focused     int
	result      *Result
	err         string
}

func NewFormDialog(id, title, description string, fields []Field) *FormDialog {
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		ti := textinput.New()
		ti.Prompt = "> "
		ti.Placeholder = cmp.Or(f.Placeholder, f.Description)
		if f.Secret {
			ti.EchoMode = textinput.EchoPassword
		}
		if f.Default != "" {
			ti.SetValue(f.Default)
			ti.CursorEnd()
		}
		ti.SetVirtualCursor(true)
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	return &FormDialog{id: id, title: title, description: description, fields: fields, inputs: inputs}
}

func (d *FormDialog) ID() string { return d.id }
func (d *FormDialog) Init() tea.Cmd {
	if len(d.inputs) == 0 {
		return nil
	}
	return d.inputs[d.focused].Focus()
}

func (d *FormDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if d.focused == len(d.inputs)-1 {
				values, cmd, ok := d.collectValues()
				if ok {
					d.result = &Result{Values: values}
				}
				return d, cmd
			}
			return d, d.moveFocus(d.focused + 1)
		case "tab", "down":
			return d, d.moveFocus(d.focused + 1)
		case "shift+tab", "up":
			return d, d.moveFocus(d.focused - 1)
		case "esc", "ctrl+c":
			d.result = &Result{Cancelled: true}
			return d, nil
		}
		input, cmd := d.inputs[d.focused].Update(msg)
		d.inputs[d.focused] = input
		return d, cmd
	case tea.PasteMsg:
		input, cmd := d.inputs[d.focused].Update(msg)
		d.inputs[d.focused] = input
		return d, cmd
	}
	return d, nil
}

func (d *FormDialog) View() string {
	t := theme.ForOutput(os.Stdout)
	headerStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle()
	labelStyle := lipgloss.NewStyle()
	errStyle := lipgloss.NewStyle()
	if t.Enabled {
		headerStyle = t.Shell.HelpHeader
		descStyle = t.Shell.Muted
		labelStyle = t.Shell.Label
		errStyle = t.Shell.Error
	}
	lines := []string{}
	if d.title != "" {
		lines = append(lines, headerStyle.Render(d.title))
	}
	if d.description != "" {
		lines = append(lines, descStyle.Render(d.description))
	}
	for i, field := range d.fields {
		label := field.Title
		if label == "" {
			label = field.ID
		}
		showLabel := len(d.fields) > 1
		if !showLabel {
			title := strings.TrimSpace(d.title)
			showLabel = title == "" || !strings.EqualFold(strings.TrimSpace(label), title)
		}
		if showLabel {
			if field.Required {
				label += "*"
			}
			lines = append(lines, labelStyle.Render(label))
		}
		lines = append(lines, d.inputs[i].View())
	}
	if d.err != "" {
		lines = append(lines, errStyle.Render(d.err))
	}
	return strings.Join(lines, "\n")
}

func (d *FormDialog) Cursor() *tea.Cursor {
	if len(d.inputs) == 0 {
		return nil
	}
	return d.inputs[d.focused].Cursor()
}

func (d *FormDialog) Result() (*Result, bool) {
	if d.result == nil {
		return nil, false
	}
	return d.result, true
}

func (d *FormDialog) Values() (map[string]string, bool) {
	if d.result != nil {
		if d.result.Cancelled {
			return nil, false
		}
		return d.result.Values, true
	}
	values := make(map[string]string, len(d.fields))
	for i, f := range d.fields {
		value := strings.TrimSpace(d.inputs[i].Value())
		if f.Required && value == "" {
			return nil, false
		}
		values[f.ID] = value
	}
	return values, true
}

func (d *FormDialog) SetValue(id, value string) {
	for i, f := range d.fields {
		if f.ID == id {
			d.inputs[i].SetValue(value)
			d.inputs[i].CursorEnd()
		}
	}
}

func (d *FormDialog) moveFocus(next int) tea.Cmd {
	if len(d.inputs) == 0 {
		return nil
	}
	if next < 0 {
		next = len(d.inputs) - 1
	}
	if next >= len(d.inputs) {
		next = 0
	}
	if next == d.focused {
		return nil
	}
	d.inputs[d.focused].Blur()
	d.focused = next
	return d.inputs[d.focused].Focus()
}

func (d *FormDialog) collectValues() (map[string]string, tea.Cmd, bool) {
	values := make(map[string]string, len(d.fields))
	for i, field := range d.fields {
		value := strings.TrimSpace(d.inputs[i].Value())
		if field.Required && value == "" {
			d.err = "please enter a value for " + field.Title
			cmd := d.moveFocus(i)
			return nil, cmd, false
		}
		if field.Validate != nil {
			if err := field.Validate(value); err != nil {
				d.err = err.Error()
				cmd := d.moveFocus(i)
				return nil, cmd, false
			}
		}
		values[field.ID] = value
	}
	d.err = ""
	return values, nil, true
}
