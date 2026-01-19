// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hostcmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a host command, using sudo when not running as root.
func Run(name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(name, args...)
	}
	sudoArgs := append([]string{"-n", name}, args...)
	return exec.Command("sudo", sudoArgs...)
}

// RunOutput executes a host command and returns a formatted error on failure.
func RunOutput(name string, args ...string) error {
	cmd := Run(name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return fmt.Errorf("failed to run %s: %w", name, err)
	}
	return fmt.Errorf("failed to run %s: %w: %s", name, err, output)
}

// RunCapture executes a host command and returns stdout on success.
func RunCapture(name string, args ...string) (string, error) {
	cmd := Run(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, output)
	}
	return string(out), nil
}
