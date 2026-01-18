// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCodexAuth(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"token":"test"}`), 0o600); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	t.Setenv("CODEX_HOME", dir)

	auth, details, err := discoverLocalAuth("codex")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if auth == nil || auth.Provider != "codex" {
		t.Fatalf("expected codex auth bundle")
	}
	if len(auth.Files) != 1 {
		t.Fatalf("expected codex file copy")
	}
	if auth.Files[0].LocalPath != authPath {
		t.Fatalf("unexpected local path: %s", auth.Files[0].LocalPath)
	}
	if auth.Files[0].ContainerPath != "/root/.codex/auth.json" {
		t.Fatalf("unexpected container path: %s", auth.Files[0].ContainerPath)
	}
	if len(details) != 1 {
		t.Fatalf("expected details for codex auth")
	}
}

func TestDiscoverClaudeAuth(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")

	auth, details, err := discoverLocalAuth("claude")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if auth == nil || auth.Provider != "claude" {
		t.Fatalf("expected claude auth bundle")
	}
	if len(auth.Env) != 1 || auth.Env["ANTHROPIC_API_KEY"] != "secret" {
		t.Fatalf("expected ANTHROPIC_API_KEY in bundle")
	}
	if len(details) != 1 {
		t.Fatalf("expected details for claude auth")
	}
}

func TestDiscoverGeminiAuth(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "creds.json")
	if err := os.WriteFile(credsPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	t.Setenv("GEMINI_API_KEY", "gem-secret")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsPath)

	auth, details, err := discoverLocalAuth("gemini")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if auth == nil || auth.Provider != "gemini" {
		t.Fatalf("expected gemini auth bundle")
	}
	if auth.Env["GEMINI_API_KEY"] != "gem-secret" {
		t.Fatalf("expected GEMINI_API_KEY in bundle")
	}
	if auth.Env["GOOGLE_APPLICATION_CREDENTIALS"] != "/root/.config/gcloud/application_default_credentials.json" {
		t.Fatalf("unexpected credentials path: %s", auth.Env["GOOGLE_APPLICATION_CREDENTIALS"])
	}
	if len(auth.Files) != 1 {
		t.Fatalf("expected one credentials file")
	}
	if len(details) != 2 {
		t.Fatalf("expected details for gemini auth")
	}
}

func TestDiscoverAmpAuth(t *testing.T) {
	dir := t.TempDir()
	ampDir := filepath.Join(dir, "amp")
	if err := os.MkdirAll(ampDir, 0o700); err != nil {
		t.Fatalf("mkdir amp: %v", err)
	}
	authPath := filepath.Join(ampDir, "secrets.json")
	if err := os.WriteFile(authPath, []byte(`{"token":"amp"}`), 0o600); err != nil {
		t.Fatalf("write amp auth: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("AMP_API_KEY", "amp-secret")

	auth, details, err := discoverLocalAuth("ampcode")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if auth == nil || auth.Provider != "ampcode" {
		t.Fatalf("expected ampcode auth bundle")
	}
	if auth.Env["AMP_API_KEY"] != "amp-secret" {
		t.Fatalf("expected AMP_API_KEY in bundle")
	}
	if len(auth.Files) != 1 {
		t.Fatalf("expected amp secrets file")
	}
	if auth.Files[0].LocalPath != authPath {
		t.Fatalf("unexpected local path: %s", auth.Files[0].LocalPath)
	}
	if auth.Files[0].ContainerPath != "/root/.local/share/amp/secrets.json" {
		t.Fatalf("unexpected container path: %s", auth.Files[0].ContainerPath)
	}
	if len(details) != 2 {
		t.Fatalf("expected details for amp auth")
	}
}

func TestDiscoverOpenCodeAuth(t *testing.T) {
	dir := t.TempDir()
	openDir := filepath.Join(dir, "opencode")
	if err := os.MkdirAll(openDir, 0o700); err != nil {
		t.Fatalf("mkdir opencode: %v", err)
	}
	authPath := filepath.Join(openDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"token":"open"}`), 0o600); err != nil {
		t.Fatalf("write opencode auth: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", dir)

	auth, details, err := discoverLocalAuth("opencode")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if auth == nil || auth.Provider != "opencode" {
		t.Fatalf("expected opencode auth bundle")
	}
	if len(auth.Files) != 1 {
		t.Fatalf("expected opencode auth file")
	}
	if auth.Files[0].LocalPath != authPath {
		t.Fatalf("unexpected local path: %s", auth.Files[0].LocalPath)
	}
	if auth.Files[0].ContainerPath != "/root/.local/share/opencode/auth.json" {
		t.Fatalf("unexpected container path: %s", auth.Files[0].ContainerPath)
	}
	if len(details) != 1 {
		t.Fatalf("expected details for opencode auth")
	}
}
