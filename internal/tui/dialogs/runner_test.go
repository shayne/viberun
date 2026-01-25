// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"bytes"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type doneMsg struct{}

type instantDialog struct {
	id     string
	result *Result
}

func (d instantDialog) ID() string { return d.id }
func (d instantDialog) Init() tea.Cmd {
	return func() tea.Msg { return doneMsg{} }
}
func (d instantDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	if _, ok := msg.(doneMsg); ok {
		d.result = &Result{Confirmed: true}
	}
	return d, nil
}
func (d instantDialog) Result() (*Result, bool) {
	if d.result == nil {
		return nil, false
	}
	return d.result, true
}
func (d instantDialog) View() string        { return "" }
func (d instantDialog) Cursor() *tea.Cursor { return nil }

func TestRunDialogReturnsResult(t *testing.T) {
	var out bytes.Buffer
	res, err := Run(bytes.NewBuffer(nil), &out, instantDialog{id: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.Confirmed {
		t.Fatalf("expected confirmed result, got %#v", res)
	}
}

type valueDialog struct {
	id string
}

func (d valueDialog) ID() string { return d.id }
func (d valueDialog) Init() tea.Cmd {
	return nil
}
func (d valueDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	return d, nil
}
func (d valueDialog) Result() (*Result, bool) {
	return nil, false
}
func (d valueDialog) Values() (map[string]string, bool) {
	return map[string]string{"value": "ok"}, true
}
func (d valueDialog) View() string        { return "" }
func (d valueDialog) Cursor() *tea.Cursor { return nil }

func TestResultFromDialogRequiresExplicitResult(t *testing.T) {
	if res, ok := resultFromDialog(valueDialog{id: "values"}); ok || res != nil {
		t.Fatalf("expected no result until dialog submits")
	}
}
