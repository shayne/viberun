// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeClaudeSettings(t *testing.T) {
	existing := []byte(`{"theme":"dark","env":{"ANTHROPIC_API_KEY":"old"}}`)
	merged, err := mergeClaudeSettings(existing, map[string]string{
		"ANTHROPIC_API_KEY":    "new",
		"ANTHROPIC_AUTH_TOKEN": "token",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["theme"] != "dark" {
		t.Fatalf("expected theme preserved")
	}
	env, ok := out["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env map")
	}
	if env["ANTHROPIC_API_KEY"] != "new" || env["ANTHROPIC_AUTH_TOKEN"] != "token" {
		t.Fatalf("env values not merged")
	}
}

func TestMergeDotEnv(t *testing.T) {
	existing := "# comment\nGEMINI_API_KEY=old\nOTHER=keep\n"
	merged := mergeDotEnv(existing, map[string]string{
		"GEMINI_API_KEY":                 "new",
		"GOOGLE_APPLICATION_CREDENTIALS": "/home/viberun/.config/gcloud/application_default_credentials.json",
	})
	if !strings.Contains(merged, "GEMINI_API_KEY=new") {
		t.Fatalf("expected GEMINI_API_KEY updated")
	}
	if !strings.Contains(merged, "OTHER=keep") {
		t.Fatalf("expected OTHER preserved")
	}
	if !strings.Contains(merged, "GOOGLE_APPLICATION_CREDENTIALS=/home/viberun/.config/gcloud/application_default_credentials.json") {
		t.Fatalf("expected credentials appended")
	}
}
