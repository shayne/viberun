// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/shayne/viberun/internal/clipboard"
	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/target"
	"golang.org/x/term"
)

func runInteractiveMuxSession(resolved target.Resolved, gateway *gatewayClient, stream *mux.Stream, outputTail *tailBuffer) error {
	if stream == nil {
		return fmt.Errorf("missing mux stream")
	}
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(fd, state)
	}()

	stopResize := startMuxResizeWatcher(stream)

	copyDone := make(chan struct{})
	go func() {
		if outputTail != nil {
			_, _ = io.Copy(io.MultiWriter(os.Stdout, outputTail), stream)
		} else {
			_, _ = io.Copy(os.Stdout, stream)
		}
		close(copyDone)
	}()

	go func() {
		buf := make([]byte, 256)
		for {
			n, readErr := os.Stdin.Read(buf)
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

	<-copyDone
	if stopResize != nil {
		stopResize()
	}
	return nil
}

func handleClipboardImagePasteMux(resolved target.Resolved, gateway *gatewayClient, stream *mux.Stream) {
	if stream == nil {
		return
	}
	go func() {
		png, err := clipboard.ReadImagePNG()
		if err != nil {
			return
		}
		path, err := uploadClipboardImageMux(resolved, gateway, png)
		if err != nil {
			fmt.Fprintf(os.Stderr, "clipboard image upload failed: %v\n", err)
			return
		}
		_, _ = stream.Write([]byte(path + " "))
	}()
}

func uploadClipboardImageMux(resolved target.Resolved, gateway *gatewayClient, png []byte) (string, error) {
	path, err := newClipboardImagePath()
	if err != nil {
		return "", err
	}
	container := fmt.Sprintf("viberun-%s", resolved.App)
	if err := uploadContainerFile(gateway, container, path, png); err != nil {
		return "", err
	}
	return path, nil
}

func sendResizeEvent(stream *mux.Stream, rows int, cols int) {
	if stream == nil || rows <= 0 || cols <= 0 {
		return
	}
	payload, _ := json.Marshal(muxrpc.ResizeEvent{Rows: rows, Cols: cols})
	_ = stream.SendMsg(payload)
}
