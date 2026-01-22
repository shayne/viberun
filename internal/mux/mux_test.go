// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mux

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestMuxStreamData(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	server := New(right, false)
	server.Handle("echo", func(stream *Stream, _ StreamOpen) {
		go func() {
			defer stream.Close()
			_, _ = io.Copy(stream, stream)
		}()
	})
	server.Run()

	client := New(left, true)
	client.Run()
	stream, err := client.OpenStream("echo", nil)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("unexpected payload: %s", string(buf))
	}
}

func TestMuxStreamMessage(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	server := New(right, false)
	server.Handle("msg", func(stream *Stream, _ StreamOpen) {
		msg, err := stream.ReceiveMsg()
		if err != nil {
			return
		}
		_ = stream.SendMsg(msg)
	})
	server.Run()

	client := New(left, true)
	client.Run()
	stream, err := client.OpenStream("msg", nil)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer stream.Close()

	if err := stream.SendMsg([]byte("hello")); err != nil {
		t.Fatalf("send msg: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		msg, err := stream.ReceiveMsg()
		if err != nil {
			t.Errorf("receive msg: %v", err)
			return
		}
		if string(msg) != "hello" {
			t.Errorf("unexpected msg: %s", string(msg))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for msg")
	}
}
