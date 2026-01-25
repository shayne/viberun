// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

func applyPromptFlowResult(state *shellState, flow promptFlow) (string, bool) {
	switch f := flow.(type) {
	case *setupFlow:
		if f.Cancelled() {
			return "", false
		}
		if note := f.Note(); note != "" {
			return note, false
		}
		plan := f.Plan()
		if plan == nil {
			return "", false
		}
		state.setupAction = &setupAction{host: plan.Host, plan: plan}
		return "", true
	case *proxySetupFlow:
		if f.Cancelled() {
			return "", false
		}
		if note := f.Note(); note != "" {
			return note, false
		}
		plan := f.Plan()
		if plan == nil {
			return "", false
		}
		state.shellAction = &shellAction{kind: actionProxySetup, host: plan.Host, proxyPlan: plan}
		return "", true
	case *wipeFlow:
		if f.Cancelled() {
			return "", false
		}
		if note := f.Note(); note != "" {
			return note, false
		}
		plan := f.Plan()
		if plan == nil {
			return "", false
		}
		state.shellAction = &shellAction{kind: actionWipe, host: plan.Host, wipePlan: plan}
		return "", true
	case *passwordFlow:
		if f.Cancelled() {
			return "", false
		}
		plan := f.Plan()
		if plan == nil {
			return "", false
		}
		kind := actionUsersSetPassword
		if strings.EqualFold(strings.TrimSpace(plan.Action), "add") {
			kind = actionUsersAdd
		}
		state.shellAction = &shellAction{
			kind:         kind,
			host:         plan.Host,
			username:     plan.Username,
			passwordPlan: plan,
		}
		return "", true
	default:
		return "", false
	}
}
