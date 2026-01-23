// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/shayne/viberun/internal/clipboard"
	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/target"
)

func runInteractiveMuxSession(resolved target.Resolved, gateway *gatewayClient, stream *mux.Stream, outputTail *tailBuffer) error {
	if stream == nil {
		return fmt.Errorf("missing mux stream")
	}
	termIO, err := openInteractiveTerminal()
	if err != nil {
		return err
	}
	defer termIO.Restore()
	flushInputBuffer(termIO.input)
	started := time.Now()

	stopResize := startMuxResizeWatcher(stream)

	stopInput := make(chan struct{})
	inputDone := startMuxInputPump(resolved, gateway, stream, stopInput, termIO.input)

	copyDone := make(chan struct{})
	go func() {
		if outputTail != nil {
			_, _ = io.Copy(io.MultiWriter(os.Stdout, outputTail), stream)
		} else {
			_, _ = io.Copy(os.Stdout, stream)
		}
		close(copyDone)
	}()

	<-copyDone
	close(stopInput)
	if inputDone != nil {
		<-inputDone
	}
	if stopResize != nil {
		stopResize()
	}
	if outputTail != nil {
		if time.Since(started) < 2*time.Second {
			if tail := summarizeSessionOutput(outputTail.String()); tail != "" {
				return fmt.Errorf("session ended immediately: %s", tail)
			}
			return fmt.Errorf("session ended immediately")
		}
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

func summarizeSessionOutput(output string) string {
	cleaned := scrubANSI(output)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	const limit = 300
	if len(cleaned) > limit {
		cleaned = cleaned[:limit] + "..."
	}
	return cleaned
}

func scrubANSI(output string) string {
	if output == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(output))
	for i := 0; i < len(output); i++ {
		ch := output[i]
		if ch == 0x1b {
			if i+1 < len(output) && output[i+1] == '[' {
				i += 2
				for i < len(output) {
					if output[i] >= 0x40 && output[i] <= 0x7e {
						break
					}
					i++
				}
				continue
			}
			if i+1 < len(output) && output[i+1] == ']' {
				i += 2
				for i < len(output) {
					if output[i] == 0x07 {
						break
					}
					if output[i] == 0x1b && i+1 < len(output) && output[i+1] == '\\' {
						i++
						break
					}
					i++
				}
				continue
			}
			if i+1 < len(output) && output[i+1] == 'P' {
				i += 2
				for i < len(output) {
					if output[i] == 0x1b && i+1 < len(output) && output[i+1] == '\\' {
						i++
						break
					}
					i++
				}
				continue
			}
			if i+1 < len(output) {
				switch output[i+1] {
				case '(', ')', '*', '+', '-', '.', '/':
					i++
					continue
				}
			}
			continue
		}
		if ch < 0x20 && ch != '\n' && ch != '\t' {
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
