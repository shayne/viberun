// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestSetupFlow_UsesPlaceholderWhenHostEmpty(t *testing.T) {
	flow := newSetupFlow(setupFlowInput{})
	dialog := flow.Dialog().(*dialogs.FormDialog)
	if _, ok := dialog.Values(); ok {
		t.Fatalf("expected host to be required when empty")
	}
}

func TestSetupFlow_PrefillsHostWhenConfigured(t *testing.T) {
	flow := newSetupFlow(setupFlowInput{
		ExistingHost:     "root@1.2.3.4",
		AlreadyConnected: true,
	})
	flow.ApplyResult(dialogs.Result{Confirmed: true})
	flow.ApplyResult(dialogs.Result{Confirmed: false})
	dialog := flow.Dialog().(*dialogs.FormDialog)
	values, ok := dialog.Values()
	if !ok {
		t.Fatalf("expected host to be prefilled")
	}
	if values["host"] != "root@1.2.3.4" {
		t.Fatalf("expected host to be prefilled")
	}
}
