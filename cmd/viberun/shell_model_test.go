// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestHandleOSCKey_DiscardSequence(t *testing.T) {
	m := shellModel{}
	sequence := []tea.KeyPressMsg{
		keyAltRune(']'),
		keyRune('1'),
		keyRune('0'),
		keyRune(';'),
		keyRune('r'),
		keyRune('g'),
		keyRune('b'),
		keyRune(':'),
		keyRune('1'),
		keyRune('a'),
		keyRune('2'),
		keyRune('b'),
		keyRune('/'),
		keyRune('3'),
		keyRune('c'),
		keyRune('4'),
		keyRune('d'),
		{Code: 'g', Text: "g", Mod: tea.ModCtrl},
	}
	for i, msg := range sequence {
		if !m.handleOSCKey(msg) {
			t.Fatalf("expected discard for index %d (%v)", i, msg)
		}
	}
	if m.oscState != oscStateIdle {
		t.Fatalf("expected osc state reset, got %v", m.oscState)
	}
	if m.handleOSCKey(keyRune('l')) {
		t.Fatalf("expected key to pass through after terminator")
	}
}

func TestHandleOSCKey_StopsOnUnexpectedRune(t *testing.T) {
	m := shellModel{}
	if !m.handleOSCKey(keyAltRune(']')) {
		t.Fatalf("expected start of OSC to be discarded")
	}
	if m.handleOSCKey(keyRune('x')) {
		t.Fatalf("unexpected rune should not be discarded")
	}
	if m.oscState != oscStateIdle {
		t.Fatalf("expected osc state reset after unexpected rune, got %v", m.oscState)
	}
}

func TestHandleOSCKey_TimeoutAllowsKey(t *testing.T) {
	m := shellModel{
		oscState:     oscStateCode,
		oscStartedAt: time.Now().Add(-oscDiscardTimeout * 2),
	}
	if m.handleOSCKey(keyRune('l')) {
		t.Fatalf("expected key to pass through after timeout")
	}
	if m.oscState != oscStateIdle {
		t.Fatalf("expected osc state reset after timeout, got %v", m.oscState)
	}
}

func TestShellModel_DialogConsumesInput(t *testing.T) {
	state := &shellState{}
	flow := newConfirmFlow("confirm", "Confirm?", "", false)
	state.promptFlow = flow
	state.promptDialog = flow.Dialog()
	model := shellModel{state: state}
	updated, _ := model.Update(keyRune('y'))
	updated, _ = updated.(shellModel).Update(keyEnter())
	next := updated.(shellModel)
	if next.state.promptFlow != nil {
		t.Fatalf("expected flow to complete and clear")
	}
}

func TestShellModel_PromptFlowForwardsNonKeyMessages(t *testing.T) {
	dialog := &testDialog{}
	flow := &testFlow{dialog: dialog}
	state := &shellState{promptFlow: flow, promptDialog: dialog}
	model := shellModel{state: state}
	updated, _ := model.Update(testMsg{})
	next := updated.(shellModel)
	if next.state.promptFlow != nil {
		t.Fatalf("expected flow to complete after non-key message")
	}
}

type testMsg struct{}

type testDialog struct {
	result *dialogs.Result
}

func (d *testDialog) ID() string                      { return "test" }
func (d *testDialog) Init() tea.Cmd                   { return nil }
func (d *testDialog) View() string                    { return "" }
func (d *testDialog) Cursor() *tea.Cursor             { return nil }
func (d *testDialog) Result() (*dialogs.Result, bool) { return d.result, d.result != nil }

func (d *testDialog) Update(msg tea.Msg) (dialogs.Dialog, tea.Cmd) {
	if _, ok := msg.(testMsg); ok {
		d.result = &dialogs.Result{Confirmed: true}
	}
	return d, nil
}

type testFlow struct {
	dialog *testDialog
	done   bool
}

func (f *testFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *testFlow) ApplyResult(dialogs.Result) {
	f.done = true
}

func (f *testFlow) Done() bool {
	return f.done
}

func (f *testFlow) Cancelled() bool {
	return false
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyAltRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r), Mod: tea.ModAlt}
}

func keyEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}
