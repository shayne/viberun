// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestProxyFlow_NewSetupCollectsPlan(t *testing.T) {
	flow := newProxySetupFlow(proxyFlowInput{
		Host:       "root@1.2.3.4",
		PublicIP:   "1.2.3.4",
		Configured: false,
	})
	if _, ok := flow.Dialog().(*dialogs.ConfirmDialog); !ok {
		t.Fatalf("expected setup confirm dialog")
	}
	flow.ApplyResult(dialogs.Result{Confirmed: true})
	if _, ok := flow.Dialog().(*dialogs.FormDialog); !ok {
		t.Fatalf("expected domain dialog")
	}
	flow.ApplyResult(dialogs.Result{Values: map[string]string{"domain": "example.com"}})
	if _, ok := flow.Dialog().(*dialogs.FormDialog); !ok {
		t.Fatalf("expected auth dialog")
	}
	flow.ApplyResult(dialogs.Result{Values: map[string]string{"username": "admin", "password": "secret"}})
	if !flow.Done() {
		t.Fatalf("expected proxy flow to complete")
	}
	plan := flow.Plan()
	if plan == nil || plan.Domain != "example.com" || plan.PublicIP != "1.2.3.4" || plan.Username != "admin" || plan.Password != "secret" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
