// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shayne/viberun/internal/authbundle"
)

type localAuth struct {
	Provider string
	Files    []localAuthFile
	Env      map[string]string
}

type localAuthFile struct {
	LocalPath     string
	ContainerPath string
	Mode          os.FileMode
}

func discoverLocalAuth(provider string) (*localAuth, []string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "codex":
		return discoverCodexAuth()
	case "claude", "claude-code":
		return discoverClaudeAuth()
	case "gemini":
		return discoverGeminiAuth()
	case "amp", "ampcode":
		return discoverAmpAuth()
	case "opencode":
		return discoverOpenCodeAuth()
	default:
		return nil, nil, nil
	}
}

func discoverCodexAuth() (*localAuth, []string, error) {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		codexHome = filepath.Join(home, ".codex")
	}
	authPath := filepath.Join(codexHome, "auth.json")
	info, err := os.Stat(authPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, nil
	}
	auth := &localAuth{
		Provider: "codex",
		Files: []localAuthFile{
			{
				LocalPath:     authPath,
				ContainerPath: "/root/.codex/auth.json",
				Mode:          0o600,
			},
		},
	}
	details := []string{authPath}
	return auth, details, nil
}

func discoverClaudeAuth() (*localAuth, []string, error) {
	env := map[string]string{}
	details := []string{}
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); value != "" {
		env["ANTHROPIC_API_KEY"] = value
		details = append(details, "ANTHROPIC_API_KEY (env)")
	}
	if value := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")); value != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = value
		details = append(details, "ANTHROPIC_AUTH_TOKEN (env)")
	}
	if len(env) == 0 {
		return nil, nil, nil
	}
	auth := &localAuth{
		Provider: "claude",
		Env:      env,
	}
	return auth, details, nil
}

func discoverGeminiAuth() (*localAuth, []string, error) {
	env := map[string]string{}
	details := []string{}
	if value := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); value != "" {
		env["GEMINI_API_KEY"] = value
		details = append(details, "GEMINI_API_KEY (env)")
	}
	if value := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); value != "" {
		env["GOOGLE_API_KEY"] = value
		details = append(details, "GOOGLE_API_KEY (env)")
	}

	files := []localAuthFile{}
	if value := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); value != "" {
		if info, err := os.Stat(value); err == nil && info.Mode().IsRegular() {
			containerPath := "/root/.config/gcloud/application_default_credentials.json"
			files = append(files, localAuthFile{
				LocalPath:     value,
				ContainerPath: containerPath,
				Mode:          0o600,
			})
			env["GOOGLE_APPLICATION_CREDENTIALS"] = containerPath
			details = append(details, "GOOGLE_APPLICATION_CREDENTIALS (file)")
		}
	}

	if len(env) == 0 && len(files) == 0 {
		return nil, nil, nil
	}
	auth := &localAuth{
		Provider: "gemini",
		Env:      env,
		Files:    files,
	}
	return auth, details, nil
}

func discoverAmpAuth() (*localAuth, []string, error) {
	env := map[string]string{}
	details := []string{}
	if value := strings.TrimSpace(os.Getenv("AMP_API_KEY")); value != "" {
		env["AMP_API_KEY"] = value
		details = append(details, "AMP_API_KEY (env)")
	}
	dataHome, err := userXdgDataHome()
	if err != nil {
		return nil, nil, err
	}
	authPath := filepath.Join(dataHome, "amp", "secrets.json")
	files := []localAuthFile{}
	if info, err := os.Stat(authPath); err == nil && info.Mode().IsRegular() {
		files = append(files, localAuthFile{
			LocalPath:     authPath,
			ContainerPath: "/root/.local/share/amp/secrets.json",
			Mode:          0o600,
		})
		details = append(details, authPath)
	}
	if len(env) == 0 && len(files) == 0 {
		return nil, nil, nil
	}
	return &localAuth{
		Provider: "ampcode",
		Env:      env,
		Files:    files,
	}, details, nil
}

func discoverOpenCodeAuth() (*localAuth, []string, error) {
	dataHome, err := userXdgDataHome()
	if err != nil {
		return nil, nil, err
	}
	authPath := filepath.Join(dataHome, "opencode", "auth.json")
	info, err := os.Stat(authPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, nil
	}
	auth := &localAuth{
		Provider: "opencode",
		Files: []localAuthFile{
			{
				LocalPath:     authPath,
				ContainerPath: "/root/.local/share/opencode/auth.json",
				Mode:          0o600,
			},
		},
	}
	details := []string{authPath}
	return auth, details, nil
}

func userXdgDataHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share"), nil
}

func promptCopyAuth(app string, provider string, details []string) bool {
	if len(details) == 0 {
		return false
	}
	sort.Strings(details)
	for _, item := range details {
		fmt.Fprintf(os.Stdout, "Found %s\n", item)
	}
	fmt.Fprintf(os.Stdout, "Copy local %s auth into the new %s container? [Y/n]: ", provider, app)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "" && input != "y" && input != "yes" {
		return false
	}
	return true
}

func stageAuthBundle(host string, auth *localAuth) (*authbundle.Bundle, error) {
	if auth == nil {
		return nil, nil
	}
	bundle := &authbundle.Bundle{
		Provider: auth.Provider,
		Env:      auth.Env,
	}
	for _, file := range auth.Files {
		hostPath, err := uploadAuthFile(host, file.LocalPath)
		if err != nil {
			return nil, err
		}
		bundle.Files = append(bundle.Files, authbundle.File{
			HostPath:      hostPath,
			ContainerPath: file.ContainerPath,
			Mode:          int(file.Mode),
		})
	}
	return bundle, nil
}

func uploadAuthFile(host string, localPath string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	remotePath := fmt.Sprintf("/tmp/viberun-auth-%s", token)
	if err := uploadFileOverSSH(host, localPath, remotePath); err != nil {
		return "", err
	}
	return remotePath, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func encodeAuthBundle(bundle *authbundle.Bundle) (string, error) {
	if bundle == nil {
		return "", nil
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
