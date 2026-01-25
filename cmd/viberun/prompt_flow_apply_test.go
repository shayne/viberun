// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

func TestApplyPromptFlowResult_PasswordSetsAction(t *testing.T) {
	state := &shellState{}
	flow := newPasswordFlow(passwordFlowInput{Host: "root@1.2.3.4", Username: "maria", Action: "add"})
	flow.ApplyResult(dialogs.Result{Values: map[string]string{"password": "secret"}})
	note, quit := applyPromptFlowResult(state, flow)
	if note != "" {
		t.Fatalf("expected no note, got %q", note)
	}
	if !quit {
		t.Fatalf("expected quit after applying password flow")
	}
	if state.shellAction == nil {
		t.Fatalf("expected shell action to be set")
	}
	if state.shellAction.kind != actionUsersAdd {
		t.Fatalf("expected users add action, got %v", state.shellAction.kind)
	}
	if state.shellAction.passwordPlan == nil || state.shellAction.passwordPlan.Password != "secret" {
		t.Fatalf("expected password plan to be set")
	}
}
