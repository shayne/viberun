// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type oscFilteringReader struct {
	r      io.Reader
	state  oscFilterState
	buf    []byte
	oscBuf []byte

	oscEnabled atomic.Bool
	oscCh      chan string
	oscMu      sync.Mutex
	oscPending map[int][]string
}

type oscFilterState int

const (
	oscFilterIdle oscFilterState = iota
	oscFilterEsc
	oscFilterOSC
	oscFilterOSCEsc
)

func newOSCFilteringReader(r io.Reader) *oscFilteringReader {
	reader := &oscFilteringReader{
		r:          r,
		oscCh:      make(chan string, 8),
		oscPending: make(map[int][]string),
	}
	return reader
}

type oscFilteringTTY struct {
	*oscFilteringReader
	file *os.File
}

func newOSCFilteringTTY(file *os.File) *oscFilteringTTY {
	return &oscFilteringTTY{
		oscFilteringReader: newOSCFilteringReader(file),
		file:               file,
	}
}

func (o *oscFilteringTTY) Write(p []byte) (int, error) {
	return o.file.Write(p)
}

func (o *oscFilteringTTY) Close() error {
	return o.file.Close()
}

func (o *oscFilteringTTY) Fd() uintptr {
	return o.file.Fd()
}

func (o *oscFilteringTTY) Name() string {
	return o.file.Name()
}

func (o *oscFilteringReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if cap(o.buf) < len(p) {
			o.buf = make([]byte, len(p))
		}
		buf := o.buf[:len(p)]
		n, err := o.r.Read(buf)
		if n == 0 {
			return 0, err
		}
		out := o.filter(buf[:n])
		if len(out) == 0 {
			if err != nil {
				return 0, err
			}
			continue
		}
		copy(p, out)
		return len(out), nil
	}
}

func (o *oscFilteringReader) filter(in []byte) []byte {
	if !o.oscEnabled.Load() {
		o.state = oscFilterIdle
		o.oscBuf = o.oscBuf[:0]
		return in
	}
	out := make([]byte, 0, len(in))
	for _, b := range in {
		switch o.state {
		case oscFilterIdle:
			if b == 0x1b {
				o.state = oscFilterEsc
				continue
			}
			out = append(out, b)
		case oscFilterEsc:
			if b == ']' {
				o.oscBuf = append(o.oscBuf[:0], 0x1b, ']')
				o.state = oscFilterOSC
				continue
			}
			out = append(out, 0x1b)
			o.state = oscFilterIdle
			if b == 0x1b {
				o.state = oscFilterEsc
				continue
			}
			out = append(out, b)
		case oscFilterOSC:
			o.oscBuf = append(o.oscBuf, b)
			if b == 0x07 {
				o.emitOSC()
				o.state = oscFilterIdle
				continue
			}
			if b == 0x1b {
				o.state = oscFilterOSCEsc
			}
		case oscFilterOSCEsc:
			o.oscBuf = append(o.oscBuf, b)
			if b == '\\' {
				o.emitOSC()
				o.state = oscFilterIdle
				continue
			}
			o.state = oscFilterOSC
		default:
			o.state = oscFilterIdle
		}
	}
	return out
}

func (o *oscFilteringReader) emitOSC() {
	if len(o.oscBuf) == 0 {
		return
	}
	resp := string(o.oscBuf)
	o.oscBuf = o.oscBuf[:0]
	select {
	case o.oscCh <- resp:
	default:
	}
}

func (o *oscFilteringReader) WaitOSC(code int, timeout time.Duration) (string, error) {
	o.oscEnabled.Store(true)
	defer o.oscEnabled.Store(false)

	deadline := time.Now().Add(timeout)
	for {
		if resp := o.popPending(code); resp != "" {
			return resp, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", errOSCTimeout
		}
		select {
		case resp := <-o.oscCh:
			if respCode, ok := oscResponseCode(resp); ok {
				if respCode == code {
					return resp, nil
				}
				o.pushPending(respCode, resp)
				continue
			}
		case <-time.After(remaining):
			return "", errOSCTimeout
		}
	}
}

func (o *oscFilteringReader) pushPending(code int, resp string) {
	o.oscMu.Lock()
	defer o.oscMu.Unlock()
	o.oscPending[code] = append(o.oscPending[code], resp)
}

func (o *oscFilteringReader) popPending(code int) string {
	o.oscMu.Lock()
	defer o.oscMu.Unlock()
	resps := o.oscPending[code]
	if len(resps) == 0 {
		return ""
	}
	resp := resps[0]
	if len(resps) == 1 {
		delete(o.oscPending, code)
		return resp
	}
	o.oscPending[code] = resps[1:]
	return resp
}

var errOSCTimeout = errors.New("osc response timeout")

func oscResponseCode(resp string) (int, bool) {
	start := -1
	for i := 0; i < len(resp); i++ {
		if resp[i] == ']' {
			start = i + 1
			break
		}
	}
	if start == -1 || start >= len(resp) {
		return 0, false
	}
	end := start
	for end < len(resp) && resp[end] >= '0' && resp[end] <= '9' {
		end++
	}
	if end == start || end >= len(resp) || resp[end] != ';' {
		return 0, false
	}
	code, err := strconv.Atoi(resp[start:end])
	if err != nil {
		return 0, false
	}
	return code, true
}
