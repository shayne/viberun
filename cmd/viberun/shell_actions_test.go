// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
)

func TestBuildAttachArgsDefaults(t *testing.T) {
	args, err := buildAttachArgs(nil, "myapp", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"attach", "myapp"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("expected %v, got %v", want, args)
	}
}

func TestBuildAttachArgsWithOverrides(t *testing.T) {
	state := &shellState{host: "root@1.2.3.4", agent: "codex"}
	args, err := buildAttachArgs(state, "myapp", "shell")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"attach", "--shell", "--host", "root@1.2.3.4", "--agent", "codex", "myapp"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("expected %v, got %v", want, args)
	}
}

func TestBuildAttachArgsRequiresApp(t *testing.T) {
	if _, err := buildAttachArgs(nil, " ", ""); err == nil {
		t.Fatal("expected error for empty app")
	}
}
