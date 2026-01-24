// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"strings"

	"github.com/shayne/viberun/internal/ui/styles"
)

// HelpLine is a single help row (command + description).
type HelpLine struct {
	Cmd    string
	Desc   string
	Indent bool
}

// RenderHelpTable renders a help section with aligned columns and a footer.
func RenderHelpTable(header string, lines []HelpLine, s styles.Styles) string {
	rows := []string{""}
	rows = append(rows, RenderHelpSection(header, lines, s)...)
	rows = append(rows, "")
	rows = append(rows, "Run "+s.Value.Render("`help <command>`")+" for more details.")
	return strings.Join(rows, "\n")
}

// RenderHelpSection renders a section of help lines with alignment.
func RenderHelpSection(header string, lines []HelpLine, s styles.Styles) []string {
	indentPrefix := "  "
	maxWidth := 0
	for _, line := range lines {
		width := len(line.Cmd)
		if line.Indent {
			width += len(indentPrefix)
		}
		if width > maxWidth {
			maxWidth = width
		}
	}
	rows := make([]string, 0, len(lines)+1)
	rows = append(rows, s.Header.Render(header))
	for _, line := range lines {
		width := len(line.Cmd)
		if line.Indent {
			width += len(indentPrefix)
		}
		padding := maxWidth - width
		if padding < 0 {
			padding = 0
		}
		prefix := "  "
		cmd := line.Cmd + strings.Repeat(" ", padding)
		if line.Indent {
			prefix += indentPrefix
			rows = append(rows, fmt.Sprintf("%s%s  %s", prefix, s.Muted.Render(cmd), s.Muted.Render("# "+line.Desc)))
			continue
		}
		rows = append(rows, fmt.Sprintf("%s%s  %s", prefix, cmd, s.Muted.Render("# "+line.Desc)))
	}
	return rows
}
