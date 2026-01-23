// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func flushTerminalInputBuffer() {
	flushInputBuffer(nil)
}

func flushInputBuffer(_ *os.File) {
	handle, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil || handle == windows.InvalidHandle {
		return
	}
	_ = windows.FlushConsoleInputBuffer(handle)
}
