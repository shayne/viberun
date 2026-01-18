// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shayne/viberun/internal/agents"
)

const (
	customAgentCheckLines = 30
)

func checkCustomAgent(app string, containerName string, agentSpec agents.Spec, extraEnv map[string]string) error {
	runner, pkg, ok := customAgentRunner(agentSpec.Provider)
	if !ok {
		return nil
	}
	if strings.TrimSpace(pkg) == "" {
		return fmt.Errorf("custom agent provider %q is missing a package name", agentSpec.Provider)
	}
	args := customAgentCheckArgs(runner, pkg)
	if len(args) == 0 {
		return nil
	}
	output, err := dockerExecCombinedOutput(containerName, args, extraEnv)
	if err == nil {
		return nil
	}
	return formatCustomAgentError(app, agentSpec.Provider, runner, pkg, output, err)
}

func customAgentRunner(provider string) (string, string, bool) {
	normalized := strings.TrimSpace(provider)
	lowered := strings.ToLower(normalized)
	switch {
	case strings.HasPrefix(lowered, "npx:"):
		pkg := strings.TrimSpace(normalized[len("npx:"):])
		return "npx", pkg, true
	case strings.HasPrefix(lowered, "uvx:"):
		pkg := strings.TrimSpace(normalized[len("uvx:"):])
		return "uvx", pkg, true
	default:
		return "", "", false
	}
}

func customAgentCheckArgs(runner string, pkg string) []string {
	switch runner {
	case "npx":
		return []string{"npx", "-y", pkg, "--help"}
	case "uvx":
		return []string{"uvx", pkg, "--help"}
	default:
		return nil
	}
}

func dockerExecCombinedOutput(name string, command []string, env map[string]string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("command is required")
	}
	filteredEnv := map[string]string{}
	for key, value := range env {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		filteredEnv[key] = value
	}
	args := dockerExecArgs(name, command, false, filteredEnv)
	cmd := exec.Command("docker", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

func formatCustomAgentError(app string, provider string, runner string, pkg string, output string, err error) error {
	var exitNote string
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitNote = fmt.Sprintf(" (exit code %d)", exitErr.ExitCode())
	}
	output = tailLines(strings.TrimSpace(output), customAgentCheckLines)
	var builder strings.Builder
	fmt.Fprintf(&builder, "custom agent %q failed to start inside the container%s.\n", provider, exitNote)
	if output != "" {
		fmt.Fprintf(&builder, "\nOutput:\n%s\n", indentLines(output, "  "))
	}
	fmt.Fprintf(&builder, "\nFix:\n")
	fmt.Fprintf(&builder, "  - Verify the package exists and exposes a CLI binary.\n")
	fmt.Fprintf(&builder, "  - Try: viberun %s shell, then run: %s %s --help\n", app, runner, pkg)
	fmt.Fprintf(&builder, "  - If the package name is wrong, rerun with --agent %s:<package>\n", runner)
	return errors.New(builder.String())
}

func tailLines(output string, maxLines int) string {
	if maxLines <= 0 {
		return output
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func indentLines(output string, prefix string) string {
	if output == "" || prefix == "" {
		return output
	}
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
