// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dialogs

import tea "charm.land/bubbletea/v2"

type Dialog interface {
	ID() string
	Init() tea.Cmd
	Update(tea.Msg) (Dialog, tea.Cmd)
	View() string
	Cursor() *tea.Cursor
}

type Result struct {
	Cancelled bool
	Values    map[string]string
	Choice    string
	Choices   []string
	Confirmed bool
}
