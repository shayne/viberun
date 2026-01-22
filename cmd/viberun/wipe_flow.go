// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/tui"
)

func runWipeFlow(host string, wipeLocal bool) (bool, error) {
	confirm, err := tui.PromptWipeDecision(os.Stdin, os.Stdout, host)
	if err != nil {
		return false, err
	}
	if !confirm {
		return false, nil
	}
	if err := ensureDevServerSynced(host); err != nil {
		return false, err
	}
	if err := tui.PromptWipeConfirm(os.Stdin, os.Stdout); err != nil {
		return false, err
	}
	gateway, err := startGateway(host, "", nil, false)
	if err != nil {
		return false, err
	}
	defer func() { _ = gateway.Close() }()
	if err := runRemoteWipe(gateway); err != nil {
		return false, err
	}
	if wipeLocal {
		if err := config.RemoveConfigFiles(); err != nil {
			return false, err
		}
	}
	fmt.Fprintln(os.Stdout, "Wipe complete.")
	return true, nil
}
