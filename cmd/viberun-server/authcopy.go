// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shayne/viberun/internal/authbundle"
)

const (
	claudeSettingsPath = "/root/.claude/settings.json"
	geminiEnvPath      = "/root/.env"
)

func loadAuthBundleFromEnv() (*authbundle.Bundle, error) {
	raw := strings.TrimSpace(os.Getenv("VIBERUN_AUTH_BUNDLE"))
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid auth bundle encoding: %w", err)
	}
	var bundle authbundle.Bundle
	if err := json.Unmarshal(decoded, &bundle); err != nil {
		return nil, fmt.Errorf("invalid auth bundle: %w", err)
	}
	return &bundle, nil
}

func applyAuthBundle(container string, bundle *authbundle.Bundle) error {
	if bundle == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(bundle.Provider)) {
	case "", "codex":
		return applyCodexAuth(container, bundle)
	case "claude", "claude-code":
		return applyClaudeAuth(container, bundle)
	case "gemini":
		return applyGeminiAuth(container, bundle)
	default:
		return nil
	}
}

func applyCodexAuth(container string, bundle *authbundle.Bundle) error {
	if len(bundle.Files) == 0 {
		return nil
	}
	for _, file := range bundle.Files {
		if err := copyHostFileToContainer(container, file); err != nil {
			return err
		}
	}
	return nil
}

func applyClaudeAuth(container string, bundle *authbundle.Bundle) error {
	if len(bundle.Env) == 0 {
		return nil
	}
	existing, err := readContainerFile(container, claudeSettingsPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	merged, err := mergeClaudeSettings(existing, bundle.Env)
	if err != nil {
		return err
	}
	return writeContainerFile(container, claudeSettingsPath, merged, 0o600)
}

func applyGeminiAuth(container string, bundle *authbundle.Bundle) error {
	for _, file := range bundle.Files {
		if err := copyHostFileToContainer(container, file); err != nil {
			return err
		}
	}
	if len(bundle.Env) == 0 {
		return nil
	}
	existing, err := readContainerFile(container, geminiEnvPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	merged := mergeDotEnv(string(existing), bundle.Env)
	return writeContainerFile(container, geminiEnvPath, []byte(merged), 0o600)
}

func mergeClaudeSettings(existing []byte, env map[string]string) ([]byte, error) {
	doc := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &doc); err != nil {
			return nil, fmt.Errorf("invalid claude settings: %w", err)
		}
	}
	envMap := map[string]any{}
	if raw, ok := doc["env"]; ok {
		if existingMap, ok := raw.(map[string]any); ok {
			envMap = existingMap
		}
	}
	for key, value := range env {
		envMap[key] = value
	}
	doc["env"] = envMap
	return json.MarshalIndent(doc, "", "  ")
}

func mergeDotEnv(existing string, env map[string]string) string {
	lines := strings.Split(existing, "\n")
	updated := make(map[string]bool, len(env))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if value, ok := env[key]; ok {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			updated[key] = true
		}
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		if updated[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", key, env[key]))
	}
	out := strings.Join(lines, "\n")
	out = strings.TrimRight(out, "\n") + "\n"
	return out
}

func readContainerFile(container string, path string) ([]byte, error) {
	cmd := exec.Command("docker", "exec", container, "sh", "-lc", "cat "+shellQuote(path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "No such file") {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to read %s: %v", path, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func writeContainerFile(container string, path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	cmd := exec.Command("docker", "exec", "-i", container, "sh", "-lc", "umask 077; mkdir -p "+shellQuote(dir)+"; cat > "+shellQuote(path))
	cmd.Stdin = bytes.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write %s: %v", path, err)
	}
	if mode != 0 {
		chmodCmd := exec.Command("docker", "exec", container, "chmod", fmt.Sprintf("%#o", mode), path)
		if err := chmodCmd.Run(); err != nil {
			return fmt.Errorf("failed to chmod %s: %v", path, err)
		}
	}
	return nil
}

func copyHostFileToContainer(container string, file authbundle.File) error {
	if strings.TrimSpace(file.HostPath) == "" || strings.TrimSpace(file.ContainerPath) == "" {
		return nil
	}
	dir := filepath.Dir(file.ContainerPath)
	if err := exec.Command("docker", "exec", container, "mkdir", "-p", dir).Run(); err != nil {
		return fmt.Errorf("failed to create %s: %v", dir, err)
	}
	if err := exec.Command("docker", "cp", file.HostPath, fmt.Sprintf("%s:%s", container, file.ContainerPath)).Run(); err != nil {
		return fmt.Errorf("failed to copy auth file: %v", err)
	}
	if file.Mode != 0 {
		if err := exec.Command("docker", "exec", container, "chmod", fmt.Sprintf("%#o", file.Mode), file.ContainerPath).Run(); err != nil {
			return fmt.Errorf("failed to chmod %s: %v", file.ContainerPath, err)
		}
	}
	_ = os.Remove(file.HostPath)
	return nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
