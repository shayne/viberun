// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestParseBranchCommand(t *testing.T) {
	cmd, err := parseBranchCommand([]string{"list", "myapp"})
	if err != nil {
		t.Fatalf("parseBranchCommand error: %v", err)
	}
	if cmd.action != "list" || cmd.base != "myapp" {
		t.Fatalf("unexpected: %+v", cmd)
	}
}
