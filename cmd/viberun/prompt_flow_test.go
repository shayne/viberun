// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestPromptFlow_ConfirmCancel(t *testing.T) {
	flow := newConfirmFlow("delete", "Delete app?", "", false)
	if flow.Done() {
		t.Fatalf("expected flow to start incomplete")
	}
	flow.ApplyResult(dialogs.Result{Cancelled: true})
	if !flow.Cancelled() {
		t.Fatalf("expected flow to cancel")
	}
}
