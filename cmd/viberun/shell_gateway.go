// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"strings"

	"github.com/shayne/viberun/internal/target"
)

func closeShellGateway(state *shellState) {
	if state == nil {
		return
	}
	if state.appsStream != nil {
		if state.appsStream.close != nil {
			state.appsStream.close()
		}
		state.appsStream = nil
	}
	if state.appForwards != nil {
		for name, forward := range state.appForwards {
			if forward.close != nil {
				forward.close()
			}
			delete(state.appForwards, name)
		}
	}
	if state.gateway == nil {
		return
	}
	_ = state.gateway.Close()
	state.gateway = nil
	state.gatewayHost = ""
}

func resolveShellHost(state *shellState, hostArg string) (target.ResolvedHost, error) {
	if state == nil {
		return target.ResolvedHost{}, errors.New("missing shell state")
	}
	targetHost := strings.TrimSpace(hostArg)
	cfg := state.cfg
	if targetHost == "" {
		targetHost = state.host
	}
	if strings.TrimSpace(targetHost) == "" {
		return target.ResolvedHost{}, errors.New("no host configured")
	}
	return target.ResolveHost(targetHost, cfg)
}

func gatewayForCommand(state *shellState, hostArg string) (*gatewayClient, func(), error) {
	resolved, err := resolveShellHost(state, hostArg)
	if err != nil {
		return nil, func() {}, err
	}
	if state.gateway != nil && state.gatewayHost == resolved.Host {
		return state.gateway, func() {}, nil
	}
	if strings.TrimSpace(hostArg) == "" {
		return nil, func() {}, errors.New("gateway not connected")
	}
	gateway, err := startGateway(resolved.Host, strings.TrimSpace(state.agent), nil, false)
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = gateway.Close() }
	return gateway, cleanup, nil
}

func isMissingServerError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "viberun-server") {
		return false
	}
	if strings.Contains(lower, "command not found") {
		return true
	}
	if strings.Contains(lower, "not found") {
		return true
	}
	if strings.Contains(lower, "no such file") {
		return true
	}
	return false
}
