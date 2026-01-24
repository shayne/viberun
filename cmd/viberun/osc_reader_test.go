// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

func TestOSCFilteringReader_StripsOSC(t *testing.T) {
	payload := []byte("\x1b]10;rgb:1111/2222/3333\x1b\\ok")
	reader := newOSCFilteringReader(bytes.NewReader(payload))
	reader.oscEnabled.Store(true)
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected ok, got %q", string(out))
	}
}

func TestOSCFilteringReader_SplitSequence(t *testing.T) {
	reader := newOSCFilteringReader(&chunkReader{
		chunks: [][]byte{
			[]byte("\x1b]10;rgb:"),
			[]byte("aaaa/bbbb/cccc"),
			[]byte("\x1b\\done"),
		},
	})
	reader.oscEnabled.Store(true)
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(out) != "done" {
		t.Fatalf("expected done, got %q", string(out))
	}
}

func TestOSCFilteringTTYImplementsTermFile(t *testing.T) {
	tty := newOSCFilteringTTY(os.Stdin)
	if _, ok := any(tty).(term.File); !ok {
		t.Fatalf("osc filtering tty does not implement term.File")
	}
}

func TestOSCFilteringTTYImplementsCancelReaderFile(t *testing.T) {
	tty := newOSCFilteringTTY(os.Stdin)
	if _, ok := any(tty).(cancelreader.File); !ok {
		t.Fatalf("osc filtering tty does not implement cancelreader.File")
	}
}

func TestOSCFilteringReader_AllowsEscapeWhenDisabled(t *testing.T) {
	payload := []byte{0x1b, 'A'}
	reader := newOSCFilteringReader(bytes.NewReader(payload))
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatalf("expected escape preserved, got %q", string(out))
	}
}

type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.idx >= len(c.chunks) {
		return 0, io.EOF
	}
	chunk := c.chunks[c.idx]
	c.idx++
	n := copy(p, chunk)
	return n, nil
}
