// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func unixHTTPClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport}
}

func TestHostRPCSnapshotAuth(t *testing.T) {
	app := "test-auth-" + time.Now().Format("20060102150405.000000000")
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

	client := unixHTTPClient(server.hostSocket)
	resp, err := client.Post("http://unix/snapshot", "text/plain", nil)
	if err != nil {
		t.Fatalf("post snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestHostRPCSnapshotAndList(t *testing.T) {
	app := "test-list-" + time.Now().Format("20060102150405.000000000")
	server, _, err := startHostRPC(app, "container", 1234, func(string, string) (string, error) {
		return "tag", nil
	}, func(string) ([]string, error) {
		return []string{"tag1", "tag2"}, nil
	}, func(string, string, int, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("startHostRPC: %v", err)
	}
	defer func() { _ = server.Close() }()

	client := unixHTTPClient(server.hostSocket)
	req, err := http.NewRequest(http.MethodPost, "http://unix/snapshot", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post snapshot: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "tag" {
		t.Fatalf("unexpected snapshot response: %q", got)
	}

	req, err = http.NewRequest(http.MethodGet, "http://unix/snapshots", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.token)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("get snapshots: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 || lines[0] != "tag1" || lines[1] != "tag2" {
		t.Fatalf("unexpected snapshots response: %q", strings.TrimSpace(string(body)))
	}
}

func TestHostRPCRestore(t *testing.T) {
	app := "test-restore-" + time.Now().Format("20060102150405.000000000")
	called := make(chan string, 1)
	server, _, err := startHostRPC(app, "container", 4242, func(string, string) (string, error) {
		return "ref", nil
	}, func(string) ([]string, error) {
		return nil, nil
	}, func(containerName string, app string, port int, snapshotRef string) error {
		called <- strings.Join([]string{containerName, app, snapshotRef}, ":")
		if port != 4242 {
			return fmt.Errorf("unexpected port %d", port)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("startHostRPC: %v", err)
	}
	defer func() { _ = server.Close() }()

	client := unixHTTPClient(server.hostSocket)
	req, err := http.NewRequest(http.MethodPost, "http://unix/restore", strings.NewReader("tag1"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+server.token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post restore: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Fatalf("unexpected restore response: %q", got)
	}
	select {
	case got := <-called:
		expected := "container:" + app + ":tag1"
		if got != expected {
			t.Fatalf("restore not called correctly: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restore callback not invoked")
	}
}
