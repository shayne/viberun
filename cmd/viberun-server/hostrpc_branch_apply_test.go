// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostRPCBranchApplyAttachFromBranch(t *testing.T) {
	baseDir := t.TempDir()
	origBaseDir := homeVolumeBaseDir
	homeVolumeBaseDir = baseDir
	t.Cleanup(func() { homeVolumeBaseDir = origBaseDir })

	meta := branchMeta{BaseApp: "myapp", Branch: "rainbow"}
	if err := writeBranchMetaAt(homeVolumeBaseDir, "myapp--rainbow", meta); err != nil {
		t.Fatalf("write branch meta: %v", err)
	}

	socketDir, err := os.MkdirTemp("/tmp", "vbr-open-")
	if err != nil {
		t.Fatalf("temp socket dir: %v", err)
	}
	socketPath := filepath.Join(socketDir, "open.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen socket: %v", err)
	}
	defer ln.Close()

	attachApp := ""
	attachSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		attachApp = strings.TrimSpace(r.Form.Get("app"))
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = attachSrv.Serve(ln) }()
	t.Cleanup(func() { _ = attachSrv.Close() })
	t.Setenv("VIBERUN_XDG_OPEN_SOCKET", socketPath)

	gotBase := ""
	gotBranch := ""
	server := &hostRPCServer{
		token: "token",
		app:   "myapp--rainbow",
		branchApplyFn: func(base string, branch string) error {
			gotBase = base
			gotBranch = branch
			return nil
		},
	}
	server.httpServer = &http.Server{Handler: server.routes()}

	req := httptest.NewRequest(http.MethodPost, "http://unix/branch/apply?attach=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	server.routes().ServeHTTP(w, req)
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if gotBase != "myapp" || gotBranch != "rainbow" {
		t.Fatalf("unexpected apply args: %q %q", gotBase, gotBranch)
	}
	if attachApp != "myapp" {
		t.Fatalf("expected attach app myapp, got %q", attachApp)
	}
}
