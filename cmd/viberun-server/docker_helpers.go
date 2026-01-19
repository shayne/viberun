// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func runDockerCommandOutput(args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("docker command required")
	}
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return fmt.Errorf("docker %s failed: %w", args[0], err)
	}
	return fmt.Errorf("docker %s failed: %w: %s", args[0], err, output)
}
