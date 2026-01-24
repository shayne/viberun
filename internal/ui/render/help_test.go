// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"testing"

	"github.com/shayne/viberun/internal/ui/styles"
)

func TestRenderHelpTableFormatting(t *testing.T) {
	lines := []HelpLine{
		{Cmd: "foo", Desc: "first"},
		{Cmd: "bar", Desc: "second", Indent: true},
	}
	got := RenderHelpTable("Commands:", lines, styles.Styles{})
	want := "\n" +
		"Commands:\n" +
		"  foo    # first\n" +
		"    bar  # second\n" +
		"\n" +
		"Run `help <command>` for more details."
	if got != want {
		t.Fatalf("unexpected output: %q", got)
	}
}
