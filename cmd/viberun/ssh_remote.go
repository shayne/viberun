// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/shayne/viberun/internal/sshcmd"
)

func runRemoteCommand(host string, remoteArgs []string) (string, error) {
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}

func runRemoteCommandWithInput(host string, remoteArgs []string, input io.Reader) (string, error) {
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	if input != nil {
		cmd.Stdin = input
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}
