// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSanitizeHostRPCName(t *testing.T) {
	cases := map[string]string{
		"":             "app",
		" MyApp ":      "myapp",
		"hello/world":  "hello_world",
		"foo.bar":      "foo.bar",
		"a b c":        "a_b_c",
		"UPPER_CASE":   "upper_case",
		"weird!chars":  "weird_chars",
		"already-good": "already-good",
	}
	for input, expected := range cases {
		if got := sanitizeHostRPCName(input); got != expected {
			t.Fatalf("sanitizeHostRPCName(%q)=%q want %q", input, got, expected)
		}
	}
}

func TestHostRPCConfigForAppBase(t *testing.T) {
	cfg := hostRPCConfigForAppBase("myapp", "/tmp/base")
	if cfg.HostDir != "/tmp/base/myapp" {
		t.Fatalf("unexpected host dir: %q", cfg.HostDir)
	}
	if cfg.ContainerDir == "" || cfg.ContainerSocket == "" || cfg.ContainerTokenFile == "" {
		t.Fatalf("expected container paths, got %+v", cfg)
	}
}

func TestStartHostRPCSetsEnvAndCreatesFiles(t *testing.T) {
	app := "test-env-" + time.Now().Format("20060102150405.000000000")
	server, env, err := startHostRPC(app, "container", 1234, func(string, string) (string, error) {
		return "ref", nil
	}, func(string) ([]string, error) {
		return nil, nil
	}, func(string, string, int, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("startHostRPC: %v", err)
	}
	defer func() { _ = server.Close() }()

	cfg := hostRPCConfigForApp(app)
	if env["VIBERUN_HOST_RPC_SOCKET"] != cfg.ContainerSocket {
		t.Fatalf("unexpected socket env: %q", env["VIBERUN_HOST_RPC_SOCKET"])
	}
	if env["VIBERUN_HOST_RPC_TOKEN_FILE"] != cfg.ContainerTokenFile {
		t.Fatalf("unexpected token env: %q", env["VIBERUN_HOST_RPC_TOKEN_FILE"])
	}
	if _, err := os.Stat(server.hostSocket); err != nil {
		t.Fatalf("expected host socket to exist: %v", err)
	}
	if _, err := os.Stat(server.hostTokenFile); err != nil {
		t.Fatalf("expected host token file to exist: %v", err)
	}
}

func TestStartHostRPCCorrectsDirPermissions(t *testing.T) {
	original := syscall.Umask(0o077)
	defer syscall.Umask(original)

	app := "test-umask-" + time.Now().Format("20060102150405.000000000")
	server, _, err := startHostRPC(app, "container", 1234, func(string, string) (string, error) {
		return "ref", nil
	}, func(string) ([]string, error) {
		return nil, nil
	}, func(string, string, int, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("startHostRPC: %v", err)
	}
	defer func() { _ = server.Close() }()

	cfg := hostRPCConfigForApp(app)
	info, err := os.Stat(cfg.HostDir)
	if err != nil {
		t.Fatalf("stat host dir: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected host rpc dir perms 0755, got %o", info.Mode().Perm())
	}
}
