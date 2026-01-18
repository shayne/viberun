// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const hostRPCContainerDir = "/var/run/viberun-hostrpc"

type hostRPCConfig struct {
	HostDir            string
	HostSocket         string
	HostTokenFile      string
	ContainerDir       string
	ContainerSocket    string
	ContainerTokenFile string
}

type hostRPCServer struct {
	token         string
	app           string
	containerName string
	port          int
	listener      net.Listener
	httpServer    *http.Server
	hostSocket    string
	hostTokenFile string
	snapshotFn    func(containerName string, app string) (string, error)
	listFn        func(app string) ([]string, error)
	restoreFn     func(containerName string, app string, port int, snapshotRef string) error
}

func hostRPCConfigForApp(app string) hostRPCConfig {
	return hostRPCConfigForAppBase(app, "/tmp/viberun-hostrpc")
}

func hostRPCConfigForAppBase(app string, baseDir string) hostRPCConfig {
	safe := sanitizeHostRPCName(app)
	hostDir := filepath.Join(baseDir, safe)
	return hostRPCConfig{
		HostDir:            hostDir,
		HostSocket:         filepath.Join(hostDir, "rpc.sock"),
		HostTokenFile:      filepath.Join(hostDir, "token"),
		ContainerDir:       hostRPCContainerDir,
		ContainerSocket:    filepath.Join(hostRPCContainerDir, "rpc.sock"),
		ContainerTokenFile: filepath.Join(hostRPCContainerDir, "token"),
	}
}

func sanitizeHostRPCName(app string) string {
	value := strings.TrimSpace(app)
	if value == "" {
		return "app"
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func ensureHostRPCDir(app string) error {
	cfg := hostRPCConfigForApp(app)
	if err := os.MkdirAll(cfg.HostDir, 0o755); err != nil {
		return fmt.Errorf("failed to create host rpc dir: %w", err)
	}
	if err := os.Chmod(cfg.HostDir, 0o755); err != nil {
		return fmt.Errorf("failed to set host rpc dir permissions: %w", err)
	}
	return nil
}

func startHostRPC(app string, containerName string, port int, snapshotFn func(containerName string, app string) (string, error), listFn func(app string) ([]string, error), restoreFn func(containerName string, app string, port int, snapshotRef string) error) (*hostRPCServer, map[string]string, error) {
	cfg := hostRPCConfigForApp(app)
	if err := ensureHostRPCDir(app); err != nil {
		return nil, nil, err
	}
	_ = os.Remove(cfg.HostSocket)
	token, err := randomHostRPCToken()
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(cfg.HostTokenFile, []byte(token+"\n"), 0o644); err != nil {
		return nil, nil, fmt.Errorf("failed to write host rpc token: %w", err)
	}
	listener, err := net.Listen("unix", cfg.HostSocket)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen on host rpc socket: %w", err)
	}
	if err := os.Chmod(cfg.HostSocket, 0o666); err != nil {
		_ = listener.Close()
		return nil, nil, fmt.Errorf("failed to set host rpc socket permissions: %w", err)
	}
	server := &hostRPCServer{
		token:         token,
		app:           app,
		containerName: containerName,
		port:          port,
		listener:      listener,
		hostSocket:    cfg.HostSocket,
		hostTokenFile: cfg.HostTokenFile,
		snapshotFn:    snapshotFn,
		listFn:        listFn,
		restoreFn:     restoreFn,
	}
	server.httpServer = &http.Server{Handler: server.routes()}
	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	env := map[string]string{
		"VIBERUN_HOST_RPC_SOCKET":     cfg.ContainerSocket,
		"VIBERUN_HOST_RPC_TOKEN_FILE": cfg.ContainerTokenFile,
	}
	return server, env, nil
}

func (s *hostRPCServer) Close() error {
	if s == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.hostSocket)
	_ = os.Remove(s.hostTokenFile)
	return nil
}

func (s *hostRPCServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot", s.handleSnapshot)
	mux.HandleFunc("/snapshots", s.handleSnapshots)
	mux.HandleFunc("/restore", s.handleRestore)
	mux.HandleFunc("/healthz", s.handleHealth)
	return mux
}

func (s *hostRPCServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *hostRPCServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ref, err := s.snapshotFn(s.containerName, s.app)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte(ref + "\n"))
}

func (s *hostRPCServer) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tags, err := s.listFn(s.app)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, tag := range tags {
		_, _ = w.Write([]byte(tag + "\n"))
	}
}

func (s *hostRPCServer) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(string(body))
	if ref == "" {
		http.Error(w, "snapshot ref required", http.StatusBadRequest)
		return
	}
	if s.restoreFn == nil {
		http.Error(w, "restore not available", http.StatusNotImplemented)
		return
	}
	resolved, err := resolveSnapshotRef(s.app, ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.restoreFn(s.containerName, s.app, s.port, resolved); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errRestoreInProgress) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *hostRPCServer) authorized(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if provided == "" || s.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}

func randomHostRPCToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate host rpc token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
