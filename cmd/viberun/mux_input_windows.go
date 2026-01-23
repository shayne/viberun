// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package main

import (
	"os"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/target"
)

func startMuxInputPump(resolved target.Resolved, gateway *gatewayClient, stream *mux.Stream, _ <-chan struct{}, input *os.File) <-chan struct{} {
	go func() {
		if input == nil {
			input = os.Stdin
		}
		buf := make([]byte, 256)
		for {
			n, readErr := input.Read(buf)
			writeFailed := false
			if n > 0 {
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
			}
			if readErr != nil || writeFailed {
				break
			}
		}
	}()
	return nil
}
