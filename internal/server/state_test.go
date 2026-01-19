// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"path/filepath"
	"testing"
)

func TestStateAssignPort(t *testing.T) {
	state := State{}
	port := state.AssignPort("app-one")
	if port != basePort {
		t.Fatalf("expected base port %d, got %d", basePort, port)
	}

	port = state.AssignPort("app-two")
	if port != basePort+1 {
		t.Fatalf("expected second port %d, got %d", basePort+1, port)
	}

	state.SetPort("app-custom", 9000)
	port = state.AssignPort("app-three")
	if port != basePort+2 {
		t.Fatalf("expected next port %d, got %d", basePort+2, port)
	}
}

func TestStateLoadSave(t *testing.T) {
	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "viberun", "server-state.json")
	t.Setenv("VIBERUN_STATE_PATH", statePath)

	state, path, err := LoadState()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.Ports) != 0 {
		t.Fatalf("expected empty state")
	}
	if path != statePath {
		t.Fatalf("unexpected state path: %s", path)
	}

	state.AssignPort("app-one")
	if err := SaveState(path, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, _, err := LoadState()
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	port, ok := loaded.PortForApp("app-one")
	if !ok || port != basePort {
		t.Fatalf("expected saved port %d, got %d (ok=%v)", basePort, port, ok)
	}
}

func TestStateRemoveApp(t *testing.T) {
	state := State{Ports: map[string]int{
		"app-a": 8080,
		"app-b": 8081,
	}}
	if removed := state.RemoveApp("missing"); removed {
		t.Fatalf("expected missing app to return false")
	}
	if removed := state.RemoveApp("app-a"); !removed {
		t.Fatalf("expected existing app to be removed")
	}
	if _, ok := state.Ports["app-a"]; ok {
		t.Fatalf("expected app-a to be removed")
	}
}
