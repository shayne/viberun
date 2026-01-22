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
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/sshcmd"
)

type gatewayClient struct {
	mux     *mux.Mux
	conn    io.ReadWriteCloser
	cmd     *exec.Cmd
	control *mux.Stream

	mu       sync.Mutex
	nextID   int64
	pending  map[string]chan rpcResult
	openOnce sync.Once
}

type rpcResult struct {
	resp muxrpc.Response
	err  error
}

type sshConn struct {
	r io.Reader
	w io.Writer
	c io.Closer
}

func (c *sshConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *sshConn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *sshConn) Close() error                { return c.c.Close() }

func startGateway(host string, agentProvider string, extraEnv map[string]string, forwardAgent bool) (*gatewayClient, error) {
	remoteArgs := []string{"viberun-server", "gateway"}
	if strings.TrimSpace(agentProvider) != "" {
		remoteArgs = append(remoteArgs, "--agent", agentProvider)
	}
	remoteArgs = prependEnv(remoteArgs, extraEnv)
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	if forwardAgent {
		sshArgs = append([]string{"-A"}, sshArgs...)
	}
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	stderr := &bytes.Buffer{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	if err := cmd.Start(); err != nil {
		return nil, withGatewayStderr(err, stderr)
	}
	conn := &sshConn{r: stdout, w: stdin, c: stdin}
	m := mux.New(conn, true)
	m.Run()
	control, err := m.OpenStream("control", nil)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, withGatewayStderr(err, stderr)
	}
	client := &gatewayClient{
		mux:     m,
		conn:    conn,
		cmd:     cmd,
		control: control,
		pending: map[string]chan rpcResult{},
	}
	go client.readResponses()
	return client, nil
}

func withGatewayStderr(err error, stderr *bytes.Buffer) error {
	if err == nil {
		return nil
	}
	if stderr == nil {
		return err
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		return err
	}
	return fmt.Errorf("%s", msg)
}

func (g *gatewayClient) Close() error {
	if g == nil {
		return nil
	}
	_ = g.control.Close()
	_ = g.mux.Close()
	if g.cmd != nil && g.cmd.Process != nil {
		_ = g.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- g.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = g.cmd.Process.Kill()
		}
	}
	return nil
}

func (g *gatewayClient) readResponses() {
	for {
		msg, err := g.control.ReceiveMsg()
		if err != nil {
			g.failAll(err)
			return
		}
		var resp muxrpc.Response
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}
		g.mu.Lock()
		ch := g.pending[resp.ID]
		if ch != nil {
			delete(g.pending, resp.ID)
		}
		g.mu.Unlock()
		if ch != nil {
			ch <- rpcResult{resp: resp}
			close(ch)
		}
	}
}

func (g *gatewayClient) failAll(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, ch := range g.pending {
		ch <- rpcResult{err: err}
		close(ch)
		delete(g.pending, id)
	}
}

func (g *gatewayClient) rpc(method string, params any, result any) error {
	if g == nil {
		return errors.New("gateway not initialized")
	}
	id := g.nextRequestID()
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = encoded
	}
	req := muxrpc.Request{ID: id, Method: method, Params: raw}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	ch := make(chan rpcResult, 1)
	g.mu.Lock()
	g.pending[id] = ch
	g.mu.Unlock()
	if err := g.control.SendMsg(payload); err != nil {
		return err
	}
	res := <-ch
	if res.err != nil {
		return res.err
	}
	if res.resp.Error != "" {
		return fmt.Errorf("%s", res.resp.Error)
	}
	if result == nil {
		return nil
	}
	if len(res.resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(res.resp.Result, result)
}

func (g *gatewayClient) command(args []string, input string, env map[string]string) (string, error) {
	params := muxrpc.CommandParams{Args: args, Input: input, Env: env}
	var result muxrpc.CommandResult
	if err := g.rpc("command", params, &result); err != nil {
		return "", err
	}
	return result.Output, nil
}

func (g *gatewayClient) exec(args []string, input string, env map[string]string) (string, error) {
	params := muxrpc.ExecParams{Args: args, Input: input, Env: env}
	var result muxrpc.ExecResult
	if err := g.rpc("exec", params, &result); err != nil {
		return "", err
	}
	return result.Output, nil
}

func (g *gatewayClient) openStream(streamType string, meta any) (*mux.Stream, error) {
	return g.mux.OpenStream(streamType, meta)
}

func (g *gatewayClient) nextRequestID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	return strconv.FormatInt(g.nextID, 10)
}

func (g *gatewayClient) startOpenStream(handler func(string)) error {
	var err error
	g.openOnce.Do(func() {
		stream, openErr := g.openStream("open", nil)
		if openErr != nil {
			err = openErr
			return
		}
		go func() {
			for {
				msg, recvErr := stream.ReceiveMsg()
				if recvErr != nil {
					return
				}
				var evt muxrpc.OpenEvent
				if jsonErr := json.Unmarshal(msg, &evt); jsonErr != nil {
					continue
				}
				if strings.TrimSpace(evt.URL) == "" {
					continue
				}
				if handler != nil {
					handler(evt.URL)
				}
			}
		}()
	})
	return err
}

func prependEnv(args []string, extraEnv map[string]string) []string {
	env := map[string]string{}
	if value := strings.TrimSpace(os.Getenv("VIBERUN_AGENT_CHECK")); value != "" {
		env["VIBERUN_AGENT_CHECK"] = value
	}
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		env[key] = value
	}
	if len(env) == 0 {
		return args
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	prefix := []string{"env"}
	for _, key := range keys {
		prefix = append(prefix, key+"="+env[key])
	}
	return append(prefix, args...)
}
