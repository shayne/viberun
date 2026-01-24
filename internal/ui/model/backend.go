// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import tea "charm.land/bubbletea/v2"

// Backend executes shell commands and returns output/commands.
type Backend interface {
	Dispatch(line string) (string, tea.Cmd)
}
