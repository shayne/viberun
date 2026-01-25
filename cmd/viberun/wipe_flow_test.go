// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestWipeFlow_CollectsPlan(t *testing.T) {
	flow := newWipeFlow(wipeFlowInput{Host: "root@1.2.3.4", WipeLocal: true})
	if _, ok := flow.Dialog().(*dialogs.ConfirmDialog); !ok {
		t.Fatalf("expected confirm dialog")
	}
	flow.ApplyResult(dialogs.Result{Confirmed: true})
	if flow.Done() || flow.Cancelled() {
		t.Fatalf("expected wipe flow to continue after confirm")
	}
	if _, ok := flow.Dialog().(*dialogs.FormDialog); !ok {
		t.Fatalf("expected token dialog")
	}
	flow.ApplyResult(dialogs.Result{Values: map[string]string{"confirm": "WIPE"}})
	if !flow.Done() {
		t.Fatalf("expected wipe flow to complete")
	}
	plan := flow.Plan()
	if plan == nil || plan.Host != "root@1.2.3.4" || !plan.WipeLocal {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
