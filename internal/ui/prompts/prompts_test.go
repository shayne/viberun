// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts

import "testing"

func TestWipePromptRequiresConfirmAndToken(t *testing.T) {
	p := NewWipePrompt()
	if !p.RequiresConfirm() || !p.RequiresToken() {
		t.Fatal("wipe prompt must require confirm and WIPE token")
	}
}
