// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestMarkAppsStale(t *testing.T) {
	state := &shellState{
		appsLoaded:  true,
		appsSyncing: true,
		apps:        []appSummary{{Name: "one"}, {Name: "two"}},
	}
	markAppsStale(state)
	if state.appsLoaded {
		t.Fatalf("expected appsLoaded false")
	}
	if state.appsSyncing {
		t.Fatalf("expected appsSyncing false")
	}
	if state.apps != nil {
		t.Fatalf("expected apps cleared")
	}
}
