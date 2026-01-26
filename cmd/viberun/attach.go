// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/target"
)

// AttachSession owns the PTY lifecycle for an interactive attach.
type AttachSession struct {
	Resolved   target.Resolved
	Gateway    *gatewayClient
	PtyMeta    muxrpc.PtyMeta
	OutputTail *tailBuffer
	OpenURL    func(string) error

	startOpen func(func(muxrpc.OpenEvent)) error
	openPTY   func(muxrpc.PtyMeta) (*mux.Stream, error)
	runPTY    func(*mux.Stream) error
}

type attachSwitchError struct {
	App    string
	Action string
}

func (e *attachSwitchError) Error() string {
	return fmt.Sprintf("switching to %s", e.App)
}

func (s *AttachSession) Run() error {
	if s == nil {
		return errors.New("missing attach session")
	}
	if strings.TrimSpace(s.PtyMeta.App) == "" {
		return errors.New("missing app for attach")
	}
	startOpen := s.startOpen
	openPTY := s.openPTY
	if startOpen == nil || openPTY == nil {
		if s.Gateway == nil {
			return errors.New("gateway not connected")
		}
		if startOpen == nil {
			startOpen = s.Gateway.startOpenStream
		}
		if openPTY == nil {
			openPTY = func(meta muxrpc.PtyMeta) (*mux.Stream, error) {
				return s.Gateway.openStream("pty", meta)
			}
		}
	}
	runPTY := s.runPTY
	if runPTY == nil {
		runPTY = func(stream *mux.Stream) error {
			return runInteractiveMuxSession(s.Resolved, s.Gateway, stream, s.OutputTail)
		}
	}

	var switchReq *attachSwitchError
	var switchMu sync.Mutex
	var ptyStream *mux.Stream
	if startOpen != nil {
		if err := startOpen(func(evt muxrpc.OpenEvent) {
			if url := strings.TrimSpace(evt.URL); url != "" && s.OpenURL != nil {
				if err := s.OpenURL(url); err != nil {
					fmt.Fprintf(os.Stderr, "open url failed: %v\n", err)
				}
				return
			}
			app := strings.TrimSpace(evt.AttachApp)
			if app == "" {
				return
			}
			switchMu.Lock()
			if switchReq == nil {
				switchReq = &attachSwitchError{App: app, Action: strings.TrimSpace(evt.AttachAction)}
			}
			switchMu.Unlock()
			if ptyStream != nil {
				_ = ptyStream.Close()
			}
		}); err != nil {
			return err
		}
	}

	ptyStream, err := openPTY(s.PtyMeta)
	if err != nil {
		return err
	}
	switchMu.Lock()
	pendingSwitch := switchReq
	switchMu.Unlock()
	if pendingSwitch != nil && ptyStream != nil {
		_ = ptyStream.Close()
	}
	err = runPTY(ptyStream)
	switchMu.Lock()
	pendingSwitch = switchReq
	switchMu.Unlock()
	if pendingSwitch != nil {
		return pendingSwitch
	}
	return err
}
