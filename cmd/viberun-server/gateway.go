// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/server"
)

type gatewayServer struct {
	defaultAgent string
	openMu       sync.Mutex
	openStream   *mux.Stream
}

func (s *gatewayServer) handleControlStream(stream *mux.Stream, _ mux.StreamOpen) {
	for {
		msg, err := stream.ReceiveMsg()
		if err != nil {
			return
		}
		var req muxrpc.Request
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}
		resp := muxrpc.Response{ID: req.ID}
		switch req.Method {
		case "version":
			result, _ := json.Marshal(map[string]string{"version": versionString()})
			resp.Result = result
		case "command":
			var params muxrpc.CommandParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = err.Error()
				break
			}
			output, err := runGatewayCommand(params)
			if err != nil {
				resp.Error = err.Error()
				break
			}
			result, _ := json.Marshal(muxrpc.CommandResult{Output: output})
			resp.Result = result
		case "exec":
			var params muxrpc.ExecParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = err.Error()
				break
			}
			output, err := runGatewayExec(params)
			if err != nil {
				resp.Error = err.Error()
				break
			}
			result, _ := json.Marshal(muxrpc.ExecResult{Output: output})
			resp.Result = result
		case "ping":
			result, _ := json.Marshal(map[string]string{"ok": "true"})
			resp.Result = result
		default:
			resp.Error = "unknown method"
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		_ = stream.SendMsg(payload)
	}
}

func (s *gatewayServer) handleOpenStream(stream *mux.Stream, _ mux.StreamOpen) {
	s.openMu.Lock()
	s.openStream = stream
	s.openMu.Unlock()
	go func() {
		for {
			if _, err := stream.ReceiveMsg(); err != nil {
				break
			}
		}
		s.openMu.Lock()
		if s.openStream == stream {
			s.openStream = nil
		}
		s.openMu.Unlock()
	}()
}

func (s *gatewayServer) handleAppsStream(stream *mux.Stream, _ mux.StreamOpen) {
	done := make(chan struct{})
	go func() {
		for {
			if _, err := stream.ReceiveMsg(); err != nil {
				close(done)
				return
			}
		}
	}()
	var lastPayload []byte
	sendSnapshot := func() bool {
		snapshots, err := loadAppSnapshots()
		if err != nil {
			return true
		}
		payload, err := json.Marshal(muxrpc.AppsEvent{Apps: snapshots})
		if err != nil {
			return true
		}
		if bytes.Equal(payload, lastPayload) {
			return true
		}
		lastPayload = payload
		if err := stream.SendMsg(payload); err != nil {
			return false
		}
		return true
	}
	if !sendSnapshot() {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if !sendSnapshot() {
				return
			}
		}
	}
}

func (s *gatewayServer) sendOpenEvent(url string) {
	s.openMu.Lock()
	stream := s.openStream
	s.openMu.Unlock()
	if stream == nil || strings.TrimSpace(url) == "" {
		return
	}
	payload, _ := json.Marshal(muxrpc.OpenEvent{URL: url})
	_ = stream.SendMsg(payload)
}

func loadAppSnapshots() ([]muxrpc.AppSnapshot, error) {
	state, statePath, err := server.LoadState()
	if err != nil {
		return nil, err
	}
	stateDirty := false
	if synced, err := syncPortsFromContainers(&state); err != nil {
		return nil, fmt.Errorf("failed to sync port mappings: %w", err)
	} else if synced {
		stateDirty = true
	}
	if err := persistState(statePath, &state, &stateDirty); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(state.Ports))
	for name := range state.Ports {
		if strings.TrimSpace(name) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	snapshots := make([]muxrpc.AppSnapshot, 0, len(names))
	for _, name := range names {
		snapshots = append(snapshots, muxrpc.AppSnapshot{Name: name, Port: state.Ports[name]})
	}
	return snapshots, nil
}

func (s *gatewayServer) handlePtyStream(stream *mux.Stream, open mux.StreamOpen) {
	var meta muxrpc.PtyMeta
	if err := json.Unmarshal(open.Meta, &meta); err != nil {
		_ = stream.Close()
		return
	}
	if strings.TrimSpace(meta.Agent) == "" {
		meta.Agent = s.defaultAgent
	}
	app := strings.TrimSpace(meta.App)
	if app == "" {
		_ = stream.Close()
		return
	}
	socketPath := gatewayOpenSocketPath(app)
	openServer, err := startGatewayOpenSocket(socketPath, s.sendOpenEvent)
	if err != nil {
		_ = stream.Close()
		return
	}
	defer func() { _ = openServer.Close() }()

	args := []string{}
	if strings.TrimSpace(meta.Agent) != "" {
		args = append(args, "--agent", meta.Agent)
	}
	args = append(args, app)
	if strings.TrimSpace(meta.Action) != "" {
		args = append(args, meta.Action)
	}
	cmd := exec.Command("viberun-server", args...)
	cmd.Env = append(os.Environ(), "VIBERUN_XDG_OPEN_SOCKET="+socketPath)
	if len(meta.Env) > 0 {
		env := map[string]string{}
		for _, entry := range cmd.Env {
			if parts := strings.SplitN(entry, "=", 2); len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
		for key, value := range meta.Env {
			if strings.TrimSpace(key) == "" {
				continue
			}
			env[key] = value
		}
		merged := make([]string, 0, len(env))
		for key, value := range env {
			merged = append(merged, key+"="+value)
		}
		cmd.Env = merged
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		_ = stream.Close()
		return
	}
	defer func() { _ = ptmx.Close() }()

	resizeDone := make(chan struct{})
	go func() {
		defer close(resizeDone)
		for {
			msg, err := stream.ReceiveMsg()
			if err != nil {
				return
			}
			var evt muxrpc.ResizeEvent
			if err := json.Unmarshal(msg, &evt); err != nil {
				continue
			}
			if evt.Rows <= 0 || evt.Cols <= 0 {
				continue
			}
			_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(evt.Rows), Cols: uint16(evt.Cols)})
		}
	}()

	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(ptmx, stream)
		close(copyDone)
	}()
	go func() {
		_, _ = io.Copy(stream, ptmx)
		_ = stream.Close()
	}()

	_ = cmd.Wait()
	<-copyDone
	<-resizeDone
}

func runGateway(agent string) error {
	server := &gatewayServer{defaultAgent: strings.TrimSpace(agent)}
	conn := &stdioConn{}
	m := mux.New(conn, false)
	m.Handle("control", server.handleControlStream)
	m.Handle("open", server.handleOpenStream)
	m.Handle("apps", server.handleAppsStream)
	m.Handle("pty", server.handlePtyStream)
	m.Handle("forward", server.handleForwardStream)
	m.Handle("upload", server.handleUploadStream)
	m.Run()
	<-m.Done()
	return nil
}

type stdioConn struct{}

func (c *stdioConn) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (c *stdioConn) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (c *stdioConn) Close() error                { return nil }

func (s *gatewayServer) handleForwardStream(stream *mux.Stream, open mux.StreamOpen) {
	var meta muxrpc.ForwardMeta
	if err := json.Unmarshal(open.Meta, &meta); err != nil {
		_ = stream.Close()
		return
	}
	host := strings.TrimSpace(meta.Host)
	if host == "" {
		host = "localhost"
	}
	if meta.Port <= 0 {
		_ = stream.Close()
		return
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(meta.Port)))
	if err != nil {
		_ = stream.Close()
		return
	}
	defer func() { _ = conn.Close() }()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(conn, stream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(stream, conn)
		_ = stream.Close()
		done <- struct{}{}
	}()
	<-done
}

func (s *gatewayServer) handleUploadStream(stream *mux.Stream, open mux.StreamOpen) {
	var meta muxrpc.UploadMeta
	if err := json.Unmarshal(open.Meta, &meta); err != nil {
		_ = stream.Close()
		return
	}
	sendResult := func(err error) {
		result := muxrpc.UploadResult{}
		if err != nil {
			result.Error = err.Error()
		}
		if payload, marshalErr := json.Marshal(result); marshalErr == nil {
			_ = stream.SendMsg(payload)
		}
	}
	switch strings.ToLower(strings.TrimSpace(meta.Target)) {
	case "container":
		container := strings.TrimSpace(meta.Container)
		path := strings.TrimSpace(meta.Path)
		if container == "" || path == "" {
			sendResult(errors.New("missing container or path"))
			_ = stream.Close()
			return
		}
		cmd := exec.Command("docker", "exec", "-i", container, "sh", "-c", "cat > "+shellQuote(path))
		if meta.Sized {
			reader, writer := io.Pipe()
			cmd.Stdin = reader
			output := &bytes.Buffer{}
			cmd.Stdout = output
			cmd.Stderr = output
			if err := cmd.Start(); err != nil {
				_ = reader.Close()
				_ = writer.Close()
				sendResult(err)
				_ = stream.Close()
				return
			}
			_, copyErr := io.CopyN(writer, stream, meta.Size)
			_ = writer.Close()
			waitErr := cmd.Wait()
			if copyErr != nil {
				_ = reader.Close()
				sendResult(copyErr)
				_ = stream.Close()
				return
			}
			if waitErr != nil {
				msg := strings.TrimSpace(output.String())
				if msg == "" {
					sendResult(waitErr)
				} else {
					sendResult(fmt.Errorf("%s", msg))
				}
				_ = stream.Close()
				return
			}
			sendResult(nil)
			_ = stream.Close()
			return
		}
		cmd.Stdin = stream
		if output, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(output))
			if msg == "" {
				sendResult(err)
			} else {
				sendResult(fmt.Errorf("%s", msg))
			}
			_ = stream.Close()
			return
		}
		sendResult(nil)
		_ = stream.Close()
	case "host":
		path := strings.TrimSpace(meta.Path)
		if path == "" {
			sendResult(errors.New("missing path"))
			_ = stream.Close()
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			sendResult(err)
			_ = stream.Close()
			return
		}
		mode := os.FileMode(0o644)
		if meta.Mode != 0 {
			mode = os.FileMode(meta.Mode)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			sendResult(err)
			_ = stream.Close()
			return
		}
		if meta.Sized {
			_, err = io.CopyN(file, stream, meta.Size)
		} else {
			_, err = io.Copy(file, stream)
		}
		_ = file.Close()
		if err != nil {
			sendResult(err)
			_ = stream.Close()
			return
		}
		if meta.Mode != 0 {
			if err := os.Chmod(path, os.FileMode(meta.Mode)); err != nil {
				sendResult(err)
				_ = stream.Close()
				return
			}
		}
		sendResult(nil)
		_ = stream.Close()
	default:
		sendResult(errors.New("unknown upload target"))
		_ = stream.Close()
	}
}

func runGatewayCommand(params muxrpc.CommandParams) (string, error) {
	if len(params.Args) == 0 {
		return "", errors.New("missing command args")
	}
	if params.Args[0] == "gateway" {
		return "", errors.New("invalid command")
	}
	cmd := exec.Command("viberun-server", params.Args...)
	cmd.Env = os.Environ()
	if len(params.Env) > 0 {
		env := map[string]string{}
		for _, entry := range cmd.Env {
			if parts := strings.SplitN(entry, "=", 2); len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
		for key, value := range params.Env {
			if strings.TrimSpace(key) == "" {
				continue
			}
			env[key] = value
		}
		merged := make([]string, 0, len(env))
		for key, value := range env {
			merged = append(merged, key+"="+value)
		}
		cmd.Env = merged
	}
	if strings.TrimSpace(params.Input) != "" {
		cmd.Stdin = strings.NewReader(params.Input)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}

func runGatewayExec(params muxrpc.ExecParams) (string, error) {
	if len(params.Args) == 0 {
		return "", errors.New("missing exec args")
	}
	cmd := exec.Command(params.Args[0], params.Args[1:]...)
	cmd.Env = os.Environ()
	if len(params.Env) > 0 {
		env := map[string]string{}
		for _, entry := range cmd.Env {
			if parts := strings.SplitN(entry, "=", 2); len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
		for key, value := range params.Env {
			if strings.TrimSpace(key) == "" {
				continue
			}
			env[key] = value
		}
		merged := make([]string, 0, len(env))
		for key, value := range env {
			merged = append(merged, key+"="+value)
		}
		cmd.Env = merged
	}
	if strings.TrimSpace(params.Input) != "" {
		cmd.Stdin = strings.NewReader(params.Input)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}

func gatewayOpenSocketPath(app string) string {
	const dir = "/tmp/viberun-open"
	const suffix = ".sock"
	cleaned := strings.TrimSpace(app)
	if cleaned != "" {
		var b strings.Builder
		for _, r := range cleaned {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune(r)
			case r >= 'A' && r <= 'Z':
				b.WriteRune(r)
			case r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == '-' || r == '_':
				b.WriteRune(r)
			default:
				b.WriteRune('-')
			}
		}
		slug := strings.Trim(b.String(), "-")
		if slug != "" {
			return dir + "/" + slug + suffix
		}
	}
	return fmt.Sprintf("%s/%d-%d%s", dir, os.Getpid(), time.Now().UnixNano(), suffix)
}

func startGatewayOpenSocket(path string, onOpen func(string)) (*http.Server, error) {
	if path == "" {
		return nil, errors.New("missing socket path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o666); err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/open" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			raw := strings.TrimSpace(r.Form.Get("url"))
			if raw == "" {
				http.Error(w, "missing url", http.StatusBadRequest)
				return
			}
			onOpen(raw)
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return server, nil
}
