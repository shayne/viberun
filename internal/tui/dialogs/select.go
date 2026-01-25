// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type Option struct {
	Label string
	Value string
}

type SelectDialog struct {
	id          string
	title       string
	description string
	options     []Option
	index       int
	result      *Result
}

func NewSelectDialog(id, title, description string, options []Option, defaultValue string) *SelectDialog {
	idx := 0
	for i, opt := range options {
		if opt.Value == defaultValue {
			idx = i
			break
		}
	}
	return &SelectDialog{id: id, title: title, description: description, options: options, index: idx}
}

func (d *SelectDialog) ID() string    { return d.id }
func (d *SelectDialog) Init() tea.Cmd { return nil }

func (d *SelectDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if len(d.options) == 0 {
				return d, nil
			}
			d.index--
			if d.index < 0 {
				d.index = len(d.options) - 1
			}
			return d, nil
		case "down", "j":
			if len(d.options) == 0 {
				return d, nil
			}
			d.index++
			if d.index >= len(d.options) {
				d.index = 0
			}
			return d, nil
		case "enter":
			if len(d.options) == 0 {
				return d, nil
			}
			d.result = &Result{Choice: d.options[d.index].Value}
			return d, nil
		case "esc", "ctrl+c":
			d.result = &Result{Cancelled: true}
			return d, nil
		}
	}
	return d, nil
}

func (d *SelectDialog) View() string {
	lines := []string{}
	if d.title != "" {
		lines = append(lines, d.title)
	}
	if d.description != "" {
		lines = append(lines, d.description)
	}
	for i, opt := range d.options {
		label := opt.Label
		if label == "" {
			label = opt.Value
		}
		marker := " "
		if i == d.index {
			marker = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %d) %s", marker, i+1, label))
	}
	return strings.Join(lines, "\n")
}

func (d *SelectDialog) Cursor() *tea.Cursor { return nil }

func (d *SelectDialog) Result() (*Result, bool) {
	if d.result == nil {
		return nil, false
	}
	return d.result, true
}

func (d *SelectDialog) SubmitDefault() {
	if len(d.options) == 0 {
		return
	}
	d.result = &Result{Choice: d.options[d.index].Value}
}

type MultiSelectDialog struct {
	id          string
	title       string
	description string
	options     []Option
	index       int
	selected    map[string]bool
	result      *Result
}

func NewMultiSelectDialog(id, title, description string, options []Option, selected []string) *MultiSelectDialog {
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[value] = true
	}
	return &MultiSelectDialog{
		id:          id,
		title:       title,
		description: description,
		options:     options,
		selected:    selectedSet,
	}
}

func (d *MultiSelectDialog) ID() string { return d.id }
func (d *MultiSelectDialog) Init() tea.Cmd {
	return nil
}
func (d *MultiSelectDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if len(d.options) == 0 {
				return d, nil
			}
			d.index--
			if d.index < 0 {
				d.index = len(d.options) - 1
			}
			return d, nil
		case "down", "j":
			if len(d.options) == 0 {
				return d, nil
			}
			d.index++
			if d.index >= len(d.options) {
				d.index = 0
			}
			return d, nil
		case " ", "space":
			if len(d.options) == 0 {
				return d, nil
			}
			value := d.options[d.index].Value
			d.selected[value] = !d.selected[value]
			return d, nil
		case "enter":
			d.result = &Result{Choices: d.selectedValues()}
			return d, nil
		case "esc", "ctrl+c":
			d.result = &Result{Cancelled: true}
			return d, nil
		}
	}
	return d, nil
}
func (d *MultiSelectDialog) View() string {
	lines := []string{}
	if d.title != "" {
		lines = append(lines, d.title)
	}
	if d.description != "" {
		lines = append(lines, d.description)
	}
	for i, opt := range d.options {
		label := opt.Label
		if label == "" {
			label = opt.Value
		}
		marker := " "
		if d.selected[opt.Value] {
			marker = "x"
		}
		cursor := " "
		if i == d.index {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %d) %s", cursor, marker, i+1, label))
	}
	return strings.Join(lines, "\n")
}
func (d *MultiSelectDialog) Cursor() *tea.Cursor {
	return nil
}
func (d *MultiSelectDialog) Result() (*Result, bool) {
	if d.result == nil {
		return nil, false
	}
	return d.result, true
}
func (d *MultiSelectDialog) SubmitDefault() {
	d.result = &Result{Choices: d.selectedValues()}
}

func (d *MultiSelectDialog) selectedValues() []string {
	values := make([]string, 0, len(d.selected))
	for _, opt := range d.options {
		if d.selected[opt.Value] {
			values = append(values, opt.Value)
		}
	}
	return values
}
