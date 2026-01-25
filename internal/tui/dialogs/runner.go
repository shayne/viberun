// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

type resultProvider interface {
	Result() (*Result, bool)
}

type valueProvider interface {
	Values() (map[string]string, bool)
}

type runnerModel struct {
	dialog Dialog
	result *Result
}

func (m runnerModel) Init() tea.Cmd { return m.dialog.Init() }

func (m runnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	d, cmd := m.dialog.Update(msg)
	m.dialog = d
	if res, ok := resultFromDialog(d); ok {
		m.result = res
		return m, tea.Quit
	}
	return m, cmd
}

func (m runnerModel) View() tea.View { return tea.NewView(m.dialog.View()) }

func (m runnerModel) Cursor() *tea.Cursor { return m.dialog.Cursor() }

func Run(in io.Reader, out io.Writer, d Dialog) (*Result, error) {
	p := tea.NewProgram(runnerModel{dialog: d}, tea.WithInput(in), tea.WithOutput(out))
	m, err := p.Run()
	if err != nil {
		return nil, err
	}
	if rm, ok := m.(runnerModel); ok {
		return rm.result, nil
	}
	return nil, nil
}

func resultFromDialog(d Dialog) (*Result, bool) {
	if provider, ok := d.(resultProvider); ok {
		if res, ok := provider.Result(); ok {
			return res, true
		}
		return nil, false
	}
	if provider, ok := d.(valueProvider); ok {
		if values, ok := provider.Values(); ok {
			return &Result{Values: values}, true
		}
	}
	return nil, false
}
