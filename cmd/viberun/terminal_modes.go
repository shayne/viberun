// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"os"
	"sync"

	"golang.org/x/term"
)

type terminalModes struct {
	fd    int
	state *term.State
	once  sync.Once
}

type interactiveTerminal struct {
	input  *os.File
	output *os.File
	modes  *terminalModes
}

func openInteractiveTerminal() (*interactiveTerminal, error) {
	input := os.Stdin
	if fd := int(input.Fd()); fd <= 0 || !term.IsTerminal(fd) {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return nil, errors.New("stdin is not a terminal")
		}
		input = tty
	}
	modes, err := enterInteractiveTerminalOn(input)
	if err != nil {
		if input != os.Stdin {
			_ = input.Close()
		}
		return nil, err
	}
	return &interactiveTerminal{input: input, output: os.Stdout, modes: modes}, nil
}

func (t *interactiveTerminal) Restore() {
	if t == nil {
		return
	}
	if t.modes != nil {
		t.modes.Restore()
	}
	if t.input != nil && t.input != os.Stdin {
		_ = t.input.Close()
	}
}

func enterInteractiveTerminalOn(input *os.File) (*terminalModes, error) {
	if input == nil {
		return nil, errors.New("stdin is not a terminal")
	}
	fd := int(input.Fd())
	if fd <= 0 || !term.IsTerminal(fd) {
		return nil, errors.New("stdin is not a terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	enableTerminalFeatures()
	return &terminalModes{fd: fd, state: state}, nil
}

func (m *terminalModes) Restore() {
	if m == nil {
		return
	}
	m.once.Do(func() {
		disableTerminalFeatures()
		if m.state != nil && m.fd > 0 {
			_ = term.Restore(m.fd, m.state)
		}
	})
}

func enableTerminalFeatures() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	_, _ = os.Stdout.Write([]byte("\x1b[?2004h")) // bracketed paste
}

func disableTerminalFeatures() {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	_, _ = os.Stdout.Write([]byte("\x1b[?2004l"))
}
