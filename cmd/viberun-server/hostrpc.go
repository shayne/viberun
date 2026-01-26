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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	branchpkg "github.com/shayne/viberun/internal/branch"
	"github.com/shayne/viberun/internal/proxy"
)

const hostRPCContainerDir = "/var/run/viberun-hostrpc"

type hostRPCConfig struct {
	HostDir             string
	HostSocket          string
	HostTokenFile       string
	HostUpdateFile      string
	ContainerDir        string
	ContainerSocket     string
	ContainerTokenFile  string
	ContainerUpdateFile string
}

type hostRPCServer struct {
	token              string
	app                string
	containerName      string
	port               int
	listener           net.Listener
	httpServer         *http.Server
	hostSocket         string
	hostTokenFile      string
	snapshotFn         func(containerName string, app string) (string, error)
	listFn             func(app string) ([]string, error)
	restoreFn          func(containerName string, app string, port int, snapshotRef string) error
	branchListFn       func(base string) ([]branchMeta, error)
	branchCreateFn     func(base string, branch string) (branchMeta, error)
	branchDeleteFn     func(base string, branch string) error
	branchApplyFn      func(base string, branch string) error
	branchApplyFromApp func(app string) error
}

func hostRPCConfigForApp(app string) hostRPCConfig {
	return hostRPCConfigForAppBase(app, "/tmp/viberun-hostrpc")
}

func hostRPCConfigForAppBase(app string, baseDir string) hostRPCConfig {
	safe := sanitizeHostRPCName(app)
	hostDir := filepath.Join(baseDir, safe)
	return hostRPCConfig{
		HostDir:             hostDir,
		HostSocket:          filepath.Join(hostDir, "rpc.sock"),
		HostTokenFile:       filepath.Join(hostDir, "token"),
		HostUpdateFile:      filepath.Join(hostDir, "update.json"),
		ContainerDir:        hostRPCContainerDir,
		ContainerSocket:     filepath.Join(hostRPCContainerDir, "rpc.sock"),
		ContainerTokenFile:  filepath.Join(hostRPCContainerDir, "token"),
		ContainerUpdateFile: filepath.Join(hostRPCContainerDir, "update.json"),
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

func deleteHostRPCDir(app string) error {
	cfg := hostRPCConfigForApp(app)
	if _, err := os.Stat(cfg.HostDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(cfg.HostDir)
}

func startHostRPC(app string, containerName string, port int, snapshotFn func(containerName string, app string) (string, error), listFn func(app string) ([]string, error), restoreFn func(containerName string, app string, port int, snapshotRef string) error) (*hostRPCServer, map[string]string, error) {
	cfg := hostRPCConfigForApp(app)
	if err := ensureHostRPCDir(app); err != nil {
		return nil, nil, err
	}
	_ = os.Remove(cfg.HostSocket)
	_ = os.Remove(cfg.HostUpdateFile)
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
		token:              token,
		app:                app,
		containerName:      containerName,
		port:               port,
		listener:           listener,
		hostSocket:         cfg.HostSocket,
		hostTokenFile:      cfg.HostTokenFile,
		snapshotFn:         snapshotFn,
		listFn:             listFn,
		restoreFn:          restoreFn,
		branchListFn:       listBranchMetas,
		branchCreateFn:     createBranchEnv,
		branchDeleteFn:     deleteBranchEnv,
		branchApplyFn:      applyBranch,
		branchApplyFromApp: applyBranchForApp,
	}
	server.httpServer = &http.Server{Handler: server.routes()}
	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	return server, map[string]string{}, nil
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
	cfg := hostRPCConfigForApp(s.app)
	_ = os.Remove(cfg.HostUpdateFile)
	return nil
}

func (s *hostRPCServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/snapshot", s.handleSnapshot)
	mux.HandleFunc("/snapshots", s.handleSnapshots)
	mux.HandleFunc("/restore", s.handleRestore)
	mux.HandleFunc("/branch/list", s.handleBranchList)
	mux.HandleFunc("/branch/create", s.handleBranchCreate)
	mux.HandleFunc("/branch/delete", s.handleBranchDelete)
	mux.HandleFunc("/branch/apply", s.handleBranchApply)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/proxy/public", s.handleProxyPublic)
	mux.HandleFunc("/proxy/private", s.handleProxyPrivate)
	mux.HandleFunc("/proxy/disable", s.handleProxyDisable)
	mux.HandleFunc("/proxy/enable", s.handleProxyEnable)
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

func (s *hostRPCServer) handleBranchList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.branchListFn == nil {
		http.Error(w, "branch list not available", http.StatusNotImplemented)
		return
	}
	metas, err := s.branchListFn(s.app)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(metas) == 0 {
		base := ""
		if meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, s.app); err == nil && ok {
			base = meta.BaseApp
		} else if parts := strings.SplitN(s.app, "--", 2); len(parts) == 2 {
			base = strings.TrimSpace(parts[0])
		}
		if base != "" && base != s.app {
			if fallback, err := s.branchListFn(base); err == nil {
				metas = fallback
			}
		}
	}
	lines := make([]string, 0, len(metas))
	for _, meta := range metas {
		if strings.TrimSpace(meta.Branch) == "" {
			continue
		}
		lines = append(lines, meta.Branch)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		_, _ = w.Write([]byte("\n"))
		return
	}
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

func (s *hostRPCServer) handleBranchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.branchCreateFn == nil {
		http.Error(w, "branch create not available", http.StatusNotImplemented)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	branch := strings.TrimSpace(string(body))
	if branch == "" {
		http.Error(w, "branch name required", http.StatusBadRequest)
		return
	}
	derived, err := branchpkg.DerivedAppName(s.app, branch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowExisting := r.URL.Query().Get("allow-existing") == "1"
	if _, err := s.branchCreateFn(s.app, branch); err != nil {
		if !allowExisting || !isBranchAlreadyExistsError(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	_, _ = w.Write([]byte(derived + "\n"))
}

func (s *hostRPCServer) handleBranchDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.branchDeleteFn == nil {
		http.Error(w, "branch delete not available", http.StatusNotImplemented)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	branch := strings.TrimSpace(string(body))
	base := s.app
	if branch == "" {
		meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, s.app)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "branch name required", http.StatusBadRequest)
			return
		}
		base = meta.BaseApp
		branch = meta.Branch
	}
	if branch == "" || strings.TrimSpace(base) == "" {
		http.Error(w, "branch name required", http.StatusBadRequest)
		return
	}
	if shouldAttach(r) && base != s.app {
		if err := sendAttachRequest(base, ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := s.branchDeleteFn(base, branch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *hostRPCServer) handleBranchApply(w http.ResponseWriter, r *http.Request) {
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
	branch := strings.TrimSpace(string(body))
	base := s.app
	if branch == "" {
		meta, ok, err := readBranchMetaAt(homeVolumeBaseDir, s.app)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if ok {
			base = meta.BaseApp
			branch = meta.Branch
		}
		if branch == "" {
			if s.branchListFn == nil {
				http.Error(w, "branch list not available", http.StatusNotImplemented)
				return
			}
			metas, err := s.branchListFn(s.app)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(metas) == 1 {
				branch = metas[0].Branch
			} else if len(metas) == 0 {
				http.Error(w, fmt.Sprintf("no branches found for %s", s.app), http.StatusBadRequest)
				return
			} else {
				names := make([]string, 0, len(metas))
				for _, meta := range metas {
					if strings.TrimSpace(meta.Branch) != "" {
						names = append(names, meta.Branch)
					}
				}
				sort.Strings(names)
				http.Error(w, fmt.Sprintf("multiple branches found for %s: %s", s.app, strings.Join(names, ", ")), http.StatusBadRequest)
				return
			}
		}
	}
	if s.branchApplyFn == nil {
		http.Error(w, "branch apply not available", http.StatusNotImplemented)
		return
	}
	if branch == "" || strings.TrimSpace(base) == "" {
		http.Error(w, "branch name required", http.StatusBadRequest)
		return
	}
	if err := s.branchApplyFn(base, branch); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if shouldAttach(r) && base != s.app {
		if err := sendAttachRequest(base, ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *hostRPCServer) handleProxyPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := setProxyAccess(&cfg, s.app, proxy.AccessPublic); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := loadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncProxyWithState(cfg, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *hostRPCServer) handleProxyPrivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := setProxyAccess(&cfg, s.app, proxy.AccessPrivate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := loadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncProxyWithState(cfg, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func shouldAttach(r *http.Request) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("attach"))
	if raw == "" {
		return false
	}
	if raw == "1" {
		return true
	}
	switch strings.ToLower(raw) {
	case "true", "yes", "y":
		return true
	default:
		return false
	}
}

func sendAttachRequest(app string, action string) error {
	socketPath, ok := xdgOpenSocketPath()
	if !ok {
		return fmt.Errorf("open socket not available")
	}
	form := url.Values{}
	form.Set("app", app)
	if strings.TrimSpace(action) != "" {
		form.Set("action", action)
	}
	req, err := http.NewRequest(http.MethodPost, "http://unix/attach", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		trimmed := strings.TrimSpace(string(body))
		if trimmed == "" {
			trimmed = resp.Status
		}
		return fmt.Errorf("attach failed: %s", trimmed)
	}
	return nil
}

func (s *hostRPCServer) handleProxyDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setProxyDisabled(&cfg, s.app, true)
	if err := proxy.SaveConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := loadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncProxyWithState(cfg, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *hostRPCServer) handleProxyEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setProxyDisabled(&cfg, s.app, false)
	if err := proxy.SaveConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := loadState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := syncProxyWithState(cfg, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
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
