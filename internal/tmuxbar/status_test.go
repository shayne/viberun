// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tmuxbar

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func writeStub(t *testing.T, dir, name, contents string) string {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func envWithOverrides(overrides map[string]string) []string {
	merged := map[string]string{}
	for _, entry := range os.Environ() {
		if eq := strings.IndexByte(entry, '='); eq != -1 {
			merged[entry[:eq]] = entry[eq+1:]
		}
	}
	for key, value := range overrides {
		merged[key] = value
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+merged[key])
	}
	return out
}

func runStatus(t *testing.T, env map[string]string) string {
	root := repoRoot(t)
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-status"), "left")
	overrides := map[string]string{
		"VIBERUN_APP":   "myapp",
		"VIBERUN_AGENT": "codex",
	}
	for key, value := range env {
		overrides[key] = value
	}
	cmd.Env = envWithOverrides(overrides)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	return out.String()
}

func TestStatusLeft_AppLabelOnly(t *testing.T) {
	output := runStatus(t, nil)
	if !strings.Contains(output, "viberun myapp") {
		t.Fatalf("expected app label, got %q", output)
	}
	if !strings.Contains(output, "|") {
		t.Fatalf("expected separator, got %q", output)
	}
}

func TestClickScript_ShellCreatesWhenMissing(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	log := filepath.Join(tmp, "tmux.log")
	writeStub(t, tmp, "tmux", "#!/bin/sh\nlog=\"${VIBERUN_TMUX_LOG:-}\"\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nif [ -n \"$log\" ]; then\n  printf '%s\\n' \"$*\" >> \"$log\"\nfi\nexit 0\n")
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-click"), "shell")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":                 tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_TMUX_LOG":     log,
		"VIBERUN_TMUX_WINDOWS": "codex\n",
	})
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "new-window -n shell") {
		t.Fatalf("expected new-window, got %q", string(data))
	}
}

func TestClickScript_Detach(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	log := filepath.Join(tmp, "tmux.log")
	writeStub(t, tmp, "tmux", "#!/bin/sh\nlog=\"${VIBERUN_TMUX_LOG:-}\"\nif [ -n \"$log\" ]; then\n  printf '%s\\n' \"$*\" >> \"$log\"\nfi\nexit 0\n")
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-click"), "detach")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":             tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_TMUX_LOG": log,
	})
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "detach-client") {
		t.Fatalf("expected detach-client, got %q", string(data))
	}
}

func TestTmuxConfHasMouseAndClickBinding(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "config", "tmux.conf"))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if !strings.Contains(contents, "set -g mouse on") {
		t.Fatalf("missing mouse option")
	}
	if !strings.Contains(contents, "MouseDown1Status") {
		t.Fatalf("missing MouseDown1Status binding")
	}
	if strings.Contains(contents, "viberun-tmux-click") {
		t.Fatalf("unexpected click script binding")
	}
	if !strings.Contains(contents, "new-window -n shell") {
		t.Fatalf("missing shell window creation")
	}
	if !strings.Contains(contents, "xdg-open") {
		t.Fatalf("missing url open handler")
	}
	if !strings.Contains(contents, "select-window") {
		t.Fatalf("missing select-window fallback")
	}
	if !strings.Contains(contents, "detach-client") {
		t.Fatalf("missing detach handler")
	}
}

func TestStatusRight_IncludesDetachWhenNoPort(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-status"), "right")
	cmd.Env = envWithOverrides(map[string]string{
		"VIBERUN_HOST_PORT": "",
	})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	if !strings.Contains(out.String(), "#[range=user|detach]detach#[range=default]") {
		t.Fatalf("expected detach range, got %q", out.String())
	}
	if strings.Contains(out.String(), "ctrl-\\ to close") {
		t.Fatalf("unexpected ctrl close hint, got %q", out.String())
	}
}

func TestStatusRight_ShellButtonWhenMissing(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	writeStub(t, tmp, "tmux", "#!/bin/sh\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nexit 0\n")
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-status"), "right")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":                 tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_HOST_PORT":    "",
		"VIBERUN_TMUX_WINDOWS": "codex\n",
	})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	if !strings.Contains(out.String(), "#[range=user|shell]shell#[range=default]") {
		t.Fatalf("expected shell button, got %q", out.String())
	}
}

func TestStatusRight_NoShellButtonWhenPresent(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	writeStub(t, tmp, "tmux", "#!/bin/sh\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nexit 0\n")
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-status"), "right")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":                 tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_HOST_PORT":    "",
		"VIBERUN_TMUX_WINDOWS": "codex\nshell\n",
	})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	if strings.Contains(out.String(), "#[range=user|shell]shell#[range=default]") {
		t.Fatalf("unexpected shell button, got %q", out.String())
	}
}

func TestStatusRight_UrlClickableWhenListening(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	writeStub(t, tmp, "tmux", "#!/bin/sh\nif [ \"$1\" = list-windows ]; then\n  printf '%s' \"${VIBERUN_TMUX_WINDOWS:-}\"\n  exit 0\nfi\nexit 0\n")
	writeStub(t, tmp, "ss", "#!/bin/sh\necho 'LISTEN 0 128 0.0.0.0:8080 '\n")
	cmd := exec.Command("sh", filepath.Join(root, "bin", "viberun-tmux-status"), "right")
	cmd.Env = envWithOverrides(map[string]string{
		"PATH":                 tmp + string(os.PathListSeparator) + os.Getenv("PATH"),
		"VIBERUN_HOST_PORT":    "5555",
		"VIBERUN_WEB_PORT":     "8080",
		"VIBERUN_TMUX_WINDOWS": "codex\n",
	})
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v (%s)", err, out.String())
	}
	if !strings.Contains(out.String(), "#[range=user|url]http://localhost:5555#[range=default]") {
		t.Fatalf("expected clickable url, got %q", out.String())
	}
}
