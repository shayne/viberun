// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
)

func TestAttachSessionRunOrder(t *testing.T) {
	calls := []string{}
	session := &AttachSession{
		PtyMeta: muxrpc.PtyMeta{App: "myapp"},
		OpenURL: func(string) error { return nil },
		startOpen: func(func(string)) error {
			calls = append(calls, "open")
			return nil
		},
		openPTY: func(muxrpc.PtyMeta) (*mux.Stream, error) {
			calls = append(calls, "pty")
			return nil, nil
		},
		runPTY: func(*mux.Stream) error {
			calls = append(calls, "run")
			return nil
		},
	}
	if err := session.Run(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	want := []string{"open", "pty", "run"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected call order %v, got %v", want, calls)
	}
}

func TestAttachSessionRunMissingApp(t *testing.T) {
	session := &AttachSession{}
	if err := session.Run(); err == nil {
		t.Fatal("expected error for missing app")
	}
}

func TestAttachSessionRunOpenURL(t *testing.T) {
	called := false
	session := &AttachSession{
		PtyMeta: muxrpc.PtyMeta{App: "myapp"},
		OpenURL: func(string) error { called = true; return nil },
		startOpen: func(cb func(string)) error {
			cb("https://example.com")
			return nil
		},
		openPTY: func(muxrpc.PtyMeta) (*mux.Stream, error) {
			return nil, nil
		},
		runPTY: func(*mux.Stream) error {
			return nil
		},
	}
	if err := session.Run(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected OpenURL to be called")
	}
}
