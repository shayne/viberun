// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func containerMountSources(name string) (map[string]struct{}, error) {
	out, err := exec.Command("docker", "inspect", "-f", "{{range .Mounts}}{{.Source}}\n{{end}}", name).Output()
	if err != nil {
		return nil, err
	}
	sources := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		sources[value] = struct{}{}
	}
	return sources, nil
}

func containerHasMountSource(name string, source string) (bool, error) {
	sources, err := containerMountSources(name)
	if err != nil {
		return false, err
	}
	_, ok := sources[source]
	return ok, nil
}

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
