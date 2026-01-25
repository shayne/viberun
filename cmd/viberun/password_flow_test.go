// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestPasswordFlow_CapturesPassword(t *testing.T) {
	flow := newPasswordFlow(passwordFlowInput{Host: "root@1.2.3.4", Username: "maria", Action: "add"})
	if _, ok := flow.Dialog().(*dialogs.FormDialog); !ok {
		t.Fatalf("expected password dialog")
	}
	flow.ApplyResult(dialogs.Result{Values: map[string]string{"password": "secret"}})
	if !flow.Done() {
		t.Fatalf("expected password flow to complete")
	}
	plan := flow.Plan()
	if plan == nil || plan.Password != "secret" || plan.Username != "maria" || plan.Host != "root@1.2.3.4" || plan.Action != "add" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
