//go:build integration

// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/target"
)

func TestGatewayClipboardUpload(t *testing.T) {
	app := strings.TrimSpace(os.Getenv("VIBERUN_TEST_APP"))
	if app == "" {
		t.Skip("set VIBERUN_TEST_APP to an existing app")
	}
	host := strings.TrimSpace(os.Getenv("VIBERUN_TEST_HOST"))
	agent := strings.TrimSpace(os.Getenv("VIBERUN_TEST_AGENT"))

	cfg, _, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if host != "" {
		cfg.DefaultHost = host
	}

	resolved, err := target.Resolve(app, cfg)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}

	gateway, err := startGateway(resolved.Host, agent, nil, false)
	if err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	defer func() { _ = gateway.Close() }()

	exists, err := remoteContainerExists(gateway, resolved.App, agent)
	if err != nil {
		t.Fatalf("check container: %v", err)
	}
	if !exists {
		t.Skipf("container for %q does not exist; run `viberun` and `vibe %s` once to create it", resolved.App, resolved.App)
	}

	payload := make([]byte, 64*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("random payload: %v", err)
	}
	path, err := newClipboardImagePath()
	if err != nil {
		t.Fatalf("temp path: %v", err)
	}
	container := fmt.Sprintf("viberun-%s", resolved.App)
	if err := uploadContainerFile(gateway, container, path, payload); err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer func() {
		_, _ = gateway.exec([]string{"docker", "exec", container, "sh", "-c", "rm -f " + shellQuote(path)}, "", nil)
	}()

	sizeOut, err := gateway.exec([]string{"docker", "exec", container, "sh", "-c", "wc -c < " + shellQuote(path)}, "", nil)
	if err != nil {
		t.Fatalf("stat upload: %v", err)
	}
	fields := strings.Fields(sizeOut)
	if len(fields) == 0 {
		t.Fatalf("stat upload: empty output")
	}
	got, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("stat upload: parse size %q: %v", fields[0], err)
	}
	if got != len(payload) {
		t.Fatalf("stat upload: size mismatch got=%d want=%d", got, len(payload))
	}

	t.Logf("upload ok: %s size=%d (%s)", path, got, time.Now().Format(time.RFC3339))
}
