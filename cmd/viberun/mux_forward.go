// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/shayne/viberun/internal/muxrpc"
)

func startLocalForwardMux(gateway *gatewayClient, port int) (func(), error) {
	if gateway == nil {
		return nil, fmt.Errorf("gateway not available")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	var closeOnce sync.Once

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
				}
				continue
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				stream, err := gateway.openStream("forward", muxrpc.ForwardMeta{Host: "localhost", Port: port})
				if err != nil {
					return
				}
				defer func() { _ = stream.Close() }()
				copyDone := make(chan struct{}, 2)
				go func() {
					_, _ = io.Copy(stream, c)
					copyDone <- struct{}{}
				}()
				go func() {
					_, _ = io.Copy(c, stream)
					copyDone <- struct{}{}
				}()
				<-copyDone
			}(conn)
		}
	}()

	return func() {
		closeOnce.Do(func() {
			close(done)
			_ = listener.Close()
		})
	}, nil
}
