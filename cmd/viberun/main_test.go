// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
)

func TestEnsureRunSubcommandBootstrap(t *testing.T) {
	args := []string{"bootstrap", "root@5.161.202.241"}
	got := ensureRunSubcommand(args)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("expected %v, got %v", args, got)
	}
}

func TestEnsureRunSubcommandDefaultRun(t *testing.T) {
	args := []string{"myapp"}
	want := []string{"run", "myapp"}
	got := ensureRunSubcommand(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestEnsureRunSubcommandHelp(t *testing.T) {
	args := []string{"help"}
	want := []string{"--help"}
	got := ensureRunSubcommand(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestEnsureRunSubcommandWithAgentFlag(t *testing.T) {
	args := []string{"--agent", "codex", "myapp"}
	want := []string{"run", "--agent", "codex", "myapp"}
	got := ensureRunSubcommand(args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
