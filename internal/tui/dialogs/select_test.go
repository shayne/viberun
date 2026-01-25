// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSelectDialog_DefaultChoice(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	d := NewSelectDialog("agent", "Choose", "", options, "b")
	d.SubmitDefault()
	choice, ok := d.Result()
	if !ok || choice.Choice != "b" {
		t.Fatalf("expected default choice b")
	}
}

func TestMultiSelectDialog_DefaultChoices(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}, {Label: "C", Value: "c"}}
	d := NewMultiSelectDialog("users", "Users", "", options, []string{"a", "c"})
	d.SubmitDefault()
	result, ok := d.Result()
	if !ok {
		t.Fatalf("expected result")
	}
	if !sameStringSet(result.Choices, []string{"a", "c"}) {
		t.Fatalf("unexpected choices: %#v", result.Choices)
	}
}

func TestSelectDialog_ViewIncludesOptions(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	d := NewSelectDialog("agent", "Choose", "Pick one", options, "a")
	view := d.View()
	if !strings.Contains(view, "Choose") || !strings.Contains(view, "A") || !strings.Contains(view, "B") {
		t.Fatalf("expected view to include options, got %q", view)
	}
}

func TestSelectDialog_KeyPressDownAndEnter(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	d := NewSelectDialog("agent", "Choose", "", options, "a")
	next, _ := d.Update(keyDownSelect())
	selected, _ := next.Update(keyEnterSelect())
	result, ok := selected.(*SelectDialog).Result()
	if !ok || result.Choice != "b" {
		t.Fatalf("expected choice b, got %#v", result)
	}
}

func TestMultiSelectDialog_ToggleAndSubmit(t *testing.T) {
	options := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	d := NewMultiSelectDialog("users", "Users", "", options, nil)
	next, _ := d.Update(keySpaceSelect())
	submitted, _ := next.Update(keyEnterSelect())
	result, ok := submitted.(*MultiSelectDialog).Result()
	if !ok || len(result.Choices) != 1 || result.Choices[0] != "a" {
		t.Fatalf("expected first choice selected, got %#v", result)
	}
}

func sameStringSet(values []string, expected []string) bool {
	if len(values) != len(expected) {
		return false
	}
	seen := map[string]int{}
	for _, value := range values {
		seen[value]++
	}
	for _, value := range expected {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func keyDownSelect() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown}
}

func keyEnterSelect() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func keySpaceSelect() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ' ', Text: " "}
}
