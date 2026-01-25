// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirmDialog_DefaultYes(t *testing.T) {
	d := NewConfirmDialog("wipe", "Wipe server?", "This removes all data.", true)
	d.SubmitDefault()
	result, ok := d.Result()
	if !ok || !result.Confirmed {
		t.Fatalf("expected default yes confirmation")
	}
}

func TestConfirmDialog_ViewIncludesDescription(t *testing.T) {
	d := NewConfirmDialog("wipe", "Wipe server?", "This removes all data.", false)
	view := d.View()
	if !strings.Contains(view, "Wipe server?") || !strings.Contains(view, "This removes all data.") {
		t.Fatalf("expected view to include title and description, got %q", view)
	}
}

func TestConfirmDialog_KeyPressYesConfirms(t *testing.T) {
	d := NewConfirmDialog("wipe", "Wipe server?", "This removes all data.", false)
	updated, _ := d.Update(keyRune('y'))
	confirm := updated.(*ConfirmDialog)
	if _, ok := confirm.Result(); ok {
		t.Fatalf("expected no confirmation before enter")
	}
	updated, _ = confirm.Update(keyEnter())
	confirm = updated.(*ConfirmDialog)
	result, ok := confirm.Result()
	if !ok || !result.Confirmed {
		t.Fatalf("expected yes to confirm")
	}
}

func TestConfirmDialog_EnterUsesDefaultNo(t *testing.T) {
	d := NewConfirmDialog("wipe", "Wipe server?", "This removes all data.", false)
	updated, _ := d.Update(keyEnter())
	confirm := updated.(*ConfirmDialog)
	result, ok := confirm.Result()
	if !ok || result.Confirmed {
		t.Fatalf("expected enter to use default no")
	}
}

func TestConfirmDialog_ViewShowsCursorBlock(t *testing.T) {
	d := NewConfirmDialog("confirm", "Confirm?", "", false)
	d.Init()
	view := d.View()
	if !strings.Contains(view, "\x1b[7") {
		t.Fatalf("expected cursor block in view, got %q", view)
	}
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}
