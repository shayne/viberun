// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/target"
)

func startMuxInputPump(resolved target.Resolved, gateway *gatewayClient, stream *mux.Stream, stop <-chan struct{}, input *os.File) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if input == nil {
			input = os.Stdin
		}
		fd := int(input.Fd())
		nonblock := syscall.SetNonblock(fd, true) == nil
		if nonblock {
			defer func() {
				_ = syscall.SetNonblock(fd, false)
			}()
		}

		buf := make([]byte, 256)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, readErr := input.Read(buf)
			if readErr != nil {
				if nonblock && (errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK)) {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				return
			}
			if n == 0 {
				continue
			}
			writeFailed := false
			for i := 0; i < n; i++ {
				if buf[i] == 0x16 {
					handleClipboardImagePasteMux(resolved, gateway, stream)
					continue
				}
				if _, err := stream.Write(buf[i : i+1]); err != nil {
					writeFailed = true
					break
				}
			}
			if writeFailed {
				return
			}
		}
	}()
	return done
}
