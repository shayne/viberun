// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import (
	"strings"
	"testing"
)

func TestFormDialog_SubmitCollectsValues(t *testing.T) {
	fields := []Field{
		{ID: "host", Title: "Server login", Placeholder: "user@host"},
		{ID: "user", Title: "Username", Required: true},
	}
	d := NewFormDialog("setup", "Setup", "Connect", fields)
	d.SetValue("host", "root@1.2.3.4")
	d.SetValue("user", "admin")

	values, ok := d.Values()
	if !ok {
		t.Fatalf("expected dialog to be complete")
	}
	if values["host"] != "root@1.2.3.4" || values["user"] != "admin" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestFormDialog_ViewIncludesTitleAndField(t *testing.T) {
	fields := []Field{{ID: "host", Title: "Server login"}}
	d := NewFormDialog("setup", "Setup", "Connect", fields)
	view := d.View()
	if !strings.Contains(view, "Setup") || !strings.Contains(view, "Server login") {
		t.Fatalf("expected view to include title and field, got %q", view)
	}
}

func TestFormDialog_ViewSkipsDuplicateSingleFieldLabel(t *testing.T) {
	fields := []Field{{ID: "host", Title: "Server login", Placeholder: "user@host"}}
	d := NewFormDialog("setup", "Server login", "", fields)
	view := d.View()
	if strings.Count(view, "Server login") != 1 {
		t.Fatalf("expected view to avoid duplicate label, got %q", view)
	}
}

func TestFormDialog_UsesVirtualCursor(t *testing.T) {
	fields := []Field{{ID: "host", Title: "Server login"}}
	d := NewFormDialog("setup", "Setup", "Connect", fields)
	if c := d.Cursor(); c != nil {
		t.Fatalf("expected virtual cursor to be used")
	}
}
