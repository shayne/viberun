// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"strings"

	"github.com/shayne/viberun/internal/ui/styles"
)

// InfoLine represents a label + description row.
type InfoLine struct {
	Label string
	Desc  string
}

// RenderInfoTable renders an info table with aligned labels.
func RenderInfoTable(header string, lines []InfoLine, s styles.Styles) string {
	maxWidth := 0
	for _, line := range lines {
		width := len(line.Label)
		if width > maxWidth {
			maxWidth = width
		}
	}
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, "")
	rows = append(rows, s.Header.Render(header))
	for _, line := range lines {
		padding := maxWidth - len(line.Label)
		if padding < 0 {
			padding = 0
		}
		label := line.Label + strings.Repeat(" ", padding)
		desc := "# " + line.Desc
		rows = append(rows, fmt.Sprintf("  %s  %s", s.Value.Render(label), s.Muted.Render(desc)))
	}
	rows = append(rows, "")
	return strings.Join(rows, "\n")
}
