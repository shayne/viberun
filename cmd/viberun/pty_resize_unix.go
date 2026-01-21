// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

func startResizeWatcher(ptmx *os.File, input *os.File) func() {
	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-sigCh:
				_ = pty.InheritSize(input, ptmx)
			case <-stop:
				signal.Stop(sigCh)
				close(sigCh)
				return
			}
		}
	}()
	return func() {
		close(stop)
	}
}
