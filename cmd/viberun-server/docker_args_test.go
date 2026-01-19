// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"testing"
)

func hasPair(args []string, key string, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestDockerRunArgsIncludesHostRPCMount(t *testing.T) {
	t.Setenv("VIBERUN_XDG_OPEN_SOCKET", "")
	cfg := hostRPCConfigForApp("myapp")
	args := dockerRunArgs("viberun-myapp", "myapp", 4242, "viberun:test")

	homeCfg := homeVolumeConfigForApp("myapp")
	if !hasPair(args, "-v", fmt.Sprintf("%s:%s", homeCfg.MountDir, "/home/viberun")) {
		t.Fatalf("expected home volume mount in args: %v", args)
	}
	if !hasPair(args, "-v", fmt.Sprintf("%s:%s", cfg.HostDir, cfg.ContainerDir)) {
		t.Fatalf("expected host rpc mount in args: %v", args)
	}
	if !hasPair(args, "-e", fmt.Sprintf("VIBERUN_HOST_RPC_SOCKET=%s", cfg.ContainerSocket)) {
		t.Fatalf("expected host rpc socket env in args: %v", args)
	}
	if !hasPair(args, "-e", fmt.Sprintf("VIBERUN_HOST_RPC_TOKEN_FILE=%s", cfg.ContainerTokenFile)) {
		t.Fatalf("expected host rpc token env in args: %v", args)
	}
}
