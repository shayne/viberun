// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos
// +build darwin dragonfly freebsd linux netbsd openbsd solaris zos

package theme

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const oscTimeout = 300 * time.Millisecond

var errTimeout = errors.New("osc timeout")
var oscReadHook func(code int, timeout time.Duration) (string, error)

// SetOSCReadHook provides an optional reader for OSC responses.
// When set, it will be used instead of reading from the tty directly.
func SetOSCReadHook(h func(code int, timeout time.Duration) (string, error)) {
	oscReadHook = h
}

func queryDefaultColors() Palette {
	if !shouldQueryOSC() {
		return Palette{}
	}
	tty, closeFn := openTTY()
	if tty == nil {
		return Palette{}
	}
	if closeFn != nil {
		defer closeFn()
	}
	fd := int(tty.Fd())
	if !isForeground(fd) {
		return Palette{}
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return Palette{}
	}
	defer term.Restore(fd, state)

	fg, hasFG := queryOSCColor(tty, fd, 10)
	bg, hasBG := queryOSCColor(tty, fd, 11)
	return Palette{FG: fg, HasFG: hasFG, BG: bg, HasBG: hasBG}
}

func shouldQueryOSC() bool {
	termValue := os.Getenv("TERM")
	if termValue == "" {
		return false
	}
	if strings.HasPrefix(termValue, "screen") || strings.HasPrefix(termValue, "tmux") || strings.HasPrefix(termValue, "dumb") {
		return false
	}
	return true
}

func openTTY() (*os.File, func()) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return tty, func() { _ = tty.Close() }
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return os.Stdin, nil
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return os.Stdout, nil
	}
	return nil, nil
}

func queryOSCColor(tty *os.File, fd int, code int) (RGB, bool) {
	_, _ = fmt.Fprintf(tty, "\x1b]%d;?\x1b\\", code)
	if oscReadHook != nil {
		response, err := oscReadHook(code, oscTimeout)
		if err != nil {
			return RGB{}, false
		}
		return parseOSCResponse(response, code)
	}
	response, err := readOSCResponse(fd, oscTimeout)
	if err != nil {
		return RGB{}, false
	}
	return parseOSCResponse(response, code)
}

func readOSCResponse(fd int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	state := 0
	buf := make([]byte, 0, 64)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", errTimeout
		}
		if err := waitForData(fd, remaining); err != nil {
			return "", err
		}
		var b [1]byte
		n, err := unix.Read(fd, b[:])
		if err != nil || n == 0 {
			continue
		}
		ch := b[0]
		switch state {
		case 0:
			if ch == 0x1b {
				buf = append(buf[:0], ch)
				state = 1
			}
		case 1:
			if ch == ']' {
				buf = append(buf, ch)
				state = 2
			} else if ch == 0x1b {
				buf = append(buf[:0], ch)
			} else {
				state = 0
				buf = buf[:0]
			}
		case 2:
			buf = append(buf, ch)
			if ch == '\a' {
				return string(buf), nil
			}
			if len(buf) >= 2 && buf[len(buf)-2] == 0x1b && buf[len(buf)-1] == '\\' {
				return string(buf), nil
			}
			if len(buf) > 64 {
				return "", errTimeout
			}
		}
	}
}

func waitForData(fd int, timeout time.Duration) error {
	if timeout <= 0 {
		return errTimeout
	}
	if fd < 0 {
		return errTimeout
	}
	pollTimeout := int(timeout / time.Millisecond)
	if pollTimeout <= 0 {
		pollTimeout = 1
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, pollTimeout)
	if err != nil {
		return err
	}
	if n == 0 {
		return errTimeout
	}
	if len(fds) == 0 || (fds[0].Revents&unix.POLLIN) == 0 {
		return errTimeout
	}
	return nil
}

func isForeground(fd int) bool {
	pgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return true
	}
	return pgrp == unix.Getpgrp()
}
