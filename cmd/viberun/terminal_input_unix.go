// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/term"
)

func flushTerminalInputBuffer() {
	flushInputBuffer(os.Stdin)
}

func flushInputBuffer(input *os.File) {
	if input == nil {
		return
	}
	fd := int(input.Fd())
	if fd <= 0 || !term.IsTerminal(fd) {
		return
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		return
	}
	defer func() {
		_ = syscall.SetNonblock(fd, false)
	}()
	buf := make([]byte, 256)
	for {
		if _, err := input.Read(buf); err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				return
			}
			return
		}
	}
}
