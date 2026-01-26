// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestValidateBranchCreateArgs(t *testing.T) {
	app, branch, derived, err := validateBranchCreateArgs("myapp", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app != "myapp" || branch != "feature" || derived != "myapp--feature" {
		t.Fatalf("unexpected values: %q %q %q", app, branch, derived)
	}
}
