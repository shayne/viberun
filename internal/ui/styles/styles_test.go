// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package styles

import "testing"

func TestDefaultStyles(t *testing.T) {
	s := DefaultStyles()
	if s.Header.Render("x") == "" {
		t.Fatal("expected Header to render")
	}
	if s.Muted.Render("x") == "" {
		t.Fatal("expected Muted to render")
	}
}
