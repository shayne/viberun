// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleOSCKey_DiscardSequence(t *testing.T) {
	m := shellModel{}
	sequence := []tea.KeyMsg{
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
		{Type: tea.KeyCtrlG},
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

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func keyAltRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{r}}
}
