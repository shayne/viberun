// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/shayne/viberun/internal/mux"
	"golang.org/x/term"
)

func startMuxResizeWatcher(stream *mux.Stream) func() {
	sendCurrentSize(stream)
	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-sigCh:
				sendCurrentSize(stream)
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

func sendCurrentSize(stream *mux.Stream) {
	if stream == nil {
		return
	}
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || rows <= 0 || cols <= 0 {
		return
	}
	sendResizeEvent(stream, rows, cols)
}
