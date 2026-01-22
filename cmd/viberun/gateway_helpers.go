// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

func buildAppCommandArgs(agentProvider string, app string, actionArgs []string) []string {
	args := []string{}
	if strings.TrimSpace(agentProvider) != "" {
		args = append(args, "--agent", agentProvider)
	}
	if strings.TrimSpace(app) != "" {
		args = append(args, app)
	}
	return append(args, actionArgs...)
}
