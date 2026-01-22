// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mux

import (
	"bytes"
	"io"
	"sync"
)

type Stream struct {
	mux     *Mux
	id      uint32
	dataCh  chan []byte
	msgCh   chan []byte
	closeCh chan struct{}
	once    sync.Once
	mu      sync.Mutex
	buf     bytes.Buffer
	closed  bool
}

func newStream(m *Mux, id uint32) *Stream {
	return &Stream{
		mux:     m,
		id:      id,
		dataCh:  make(chan []byte, 8),
		msgCh:   make(chan []byte, 8),
		closeCh: make(chan struct{}),
	}
}

func (s *Stream) ID() uint32 {
	return s.id
}

func (s *Stream) Read(p []byte) (int, error) {
	for {
		s.mu.Lock()
		if s.buf.Len() > 0 {
			n, err := s.buf.Read(p)
			s.mu.Unlock()
			return n, err
		}
		if s.closed {
			s.mu.Unlock()
			return 0, io.EOF
		}
		s.mu.Unlock()
		select {
		case chunk, ok := <-s.dataCh:
			if !ok {
				return 0, io.EOF
			}
			s.mu.Lock()
			_, _ = s.buf.Write(chunk)
			s.mu.Unlock()
		case <-s.closeCh:
			return 0, io.EOF
		}
	}
}

func (s *Stream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if s.isClosed() {
		return 0, errClosed
	}
	if err := s.mux.writeFrame(frameData, s.id, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *Stream) SendMsg(p []byte) error {
	if s.isClosed() {
		return errClosed
	}
	return s.mux.writeFrame(frameMsg, s.id, p)
}

func (s *Stream) ReceiveMsg() ([]byte, error) {
	select {
	case msg, ok := <-s.msgCh:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	case <-s.closeCh:
		return nil, io.EOF
	}
}

func (s *Stream) Close() error {
	var err error
	s.once.Do(func() {
		_ = s.mux.writeFrame(frameClose, s.id, nil)
		close(s.closeCh)
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.dataCh)
		close(s.msgCh)
		s.mux.remove(s.id)
	})
	return err
}

func (s *Stream) pushData(payload []byte) {
	if s.isClosed() {
		return
	}
	buf := make([]byte, len(payload))
	copy(buf, payload)
	select {
	case s.dataCh <- buf:
	case <-s.closeCh:
	}
}

func (s *Stream) pushMsg(payload []byte) {
	if s.isClosed() {
		return
	}
	buf := make([]byte, len(payload))
	copy(buf, payload)
	select {
	case s.msgCh <- buf:
	case <-s.closeCh:
	}
}

func (s *Stream) remoteClose() {
	s.once.Do(func() {
		close(s.closeCh)
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.dataCh)
		close(s.msgCh)
	})
}

func (s *Stream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

var _ io.ReadWriteCloser = (*Stream)(nil)
