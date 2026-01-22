// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mux

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	frameOpen  = 1
	frameData  = 2
	frameMsg   = 3
	frameClose = 4
)

const maxFrameSize = 8 * 1024 * 1024

var errClosed = errors.New("mux closed")

type StreamOpen struct {
	Type string          `json:"type"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

type Handler func(*Stream, StreamOpen)

type Mux struct {
	conn     io.ReadWriteCloser
	reader   *bufio.Reader
	writer   *bufio.Writer
	client   bool
	mu       sync.Mutex
	streamMu sync.Mutex
	streams  map[uint32]*Stream
	handlers map[string]Handler
	nextID   uint32
	closed   chan struct{}
}

func New(conn io.ReadWriteCloser, client bool) *Mux {
	start := uint32(1)
	if !client {
		start = 2
	}
	return &Mux{
		conn:     conn,
		reader:   bufio.NewReader(conn),
		writer:   bufio.NewWriter(conn),
		client:   client,
		streams:  map[uint32]*Stream{},
		handlers: map[string]Handler{},
		nextID:   start,
		closed:   make(chan struct{}),
	}
}

func (m *Mux) Handle(streamType string, handler Handler) {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()
	m.handlers[streamType] = handler
}

func (m *Mux) Run() {
	go m.readLoop()
}

func (m *Mux) Close() error {
	select {
	case <-m.closed:
		return nil
	default:
		close(m.closed)
	}
	_ = m.conn.Close()
	m.streamMu.Lock()
	for _, stream := range m.streams {
		stream.remoteClose()
	}
	m.streams = map[uint32]*Stream{}
	m.streamMu.Unlock()
	return nil
}

func (m *Mux) Done() <-chan struct{} {
	return m.closed
}

func (m *Mux) OpenStream(streamType string, meta any) (*Stream, error) {
	select {
	case <-m.closed:
		return nil, errClosed
	default:
	}
	var metaRaw json.RawMessage
	if meta != nil {
		encoded, err := json.Marshal(meta)
		if err != nil {
			return nil, err
		}
		metaRaw = encoded
	}
	open := StreamOpen{Type: streamType, Meta: metaRaw}
	openPayload, err := json.Marshal(open)
	if err != nil {
		return nil, err
	}
	streamID := m.nextStreamID()
	stream := newStream(m, streamID)
	m.streamMu.Lock()
	m.streams[streamID] = stream
	m.streamMu.Unlock()
	if err := m.writeFrame(frameOpen, streamID, openPayload); err != nil {
		m.streamMu.Lock()
		delete(m.streams, streamID)
		m.streamMu.Unlock()
		return nil, err
	}
	return stream, nil
}

func (m *Mux) nextStreamID() uint32 {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()
	id := m.nextID
	m.nextID += 2
	return id
}

func (m *Mux) readLoop() {
	for {
		frameType, streamID, payload, err := m.readFrame()
		if err != nil {
			_ = m.Close()
			return
		}
		switch frameType {
		case frameOpen:
			m.handleOpen(streamID, payload)
		case frameData:
			m.handleData(streamID, payload)
		case frameMsg:
			m.handleMsg(streamID, payload)
		case frameClose:
			m.handleClose(streamID)
		}
	}
}

func (m *Mux) handleOpen(streamID uint32, payload []byte) {
	var open StreamOpen
	if err := json.Unmarshal(payload, &open); err != nil {
		return
	}
	stream := newStream(m, streamID)
	m.streamMu.Lock()
	m.streams[streamID] = stream
	handler := m.handlers[open.Type]
	m.streamMu.Unlock()
	if handler == nil {
		stream.Close()
		return
	}
	go handler(stream, open)
}

func (m *Mux) handleData(streamID uint32, payload []byte) {
	stream := m.lookup(streamID)
	if stream == nil {
		return
	}
	stream.pushData(payload)
}

func (m *Mux) handleMsg(streamID uint32, payload []byte) {
	stream := m.lookup(streamID)
	if stream == nil {
		return
	}
	stream.pushMsg(payload)
}

func (m *Mux) handleClose(streamID uint32) {
	stream := m.lookup(streamID)
	if stream == nil {
		return
	}
	stream.remoteClose()
	m.streamMu.Lock()
	delete(m.streams, streamID)
	m.streamMu.Unlock()
}

func (m *Mux) lookup(streamID uint32) *Stream {
	m.streamMu.Lock()
	defer m.streamMu.Unlock()
	return m.streams[streamID]
}

func (m *Mux) remove(streamID uint32) {
	m.streamMu.Lock()
	delete(m.streams, streamID)
	m.streamMu.Unlock()
}

func (m *Mux) writeFrame(frameType byte, streamID uint32, payload []byte) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("frame too large: %d", len(payload))
	}
	select {
	case <-m.closed:
		return errClosed
	default:
	}
	buf := make([]byte, 9)
	buf[0] = frameType
	binary.BigEndian.PutUint32(buf[1:5], streamID)
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(payload)))
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.writer.Write(buf); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := m.writer.Write(payload); err != nil {
			return err
		}
	}
	return m.writer.Flush()
}

func (m *Mux) readFrame() (byte, uint32, []byte, error) {
	header := make([]byte, 9)
	if _, err := io.ReadFull(m.reader, header); err != nil {
		return 0, 0, nil, err
	}
	frameType := header[0]
	streamID := binary.BigEndian.Uint32(header[1:5])
	length := binary.BigEndian.Uint32(header[5:9])
	if length > maxFrameSize {
		return 0, 0, nil, fmt.Errorf("frame too large: %d", length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(m.reader, payload); err != nil {
			return 0, 0, nil, err
		}
	}
	return frameType, streamID, payload, nil
}
