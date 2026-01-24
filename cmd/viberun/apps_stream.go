// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/shayne/viberun/internal/muxrpc"
)

type appsEvent struct {
	apps []appSnapshot
	err  error
}

type appsStream struct {
	updates <-chan appsEvent
	close   func()
}

func startAppsStream(gateway *gatewayClient) (*appsStream, error) {
	if gateway == nil {
		return nil, errors.New("gateway not connected")
	}
	stream, err := gateway.openStream("apps", nil)
	if err != nil {
		return nil, err
	}
	updates := make(chan appsEvent, 1)
	go func() {
		defer close(updates)
		send := func(event appsEvent) {
			select {
			case updates <- event:
				return
			default:
			}
			select {
			case <-updates:
			default:
			}
			select {
			case updates <- event:
			default:
			}
		}
		for {
			msg, err := stream.ReceiveMsg()
			if err != nil {
				send(appsEvent{err: err})
				return
			}
			var event muxrpc.AppsEvent
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			apps := make([]appSnapshot, 0, len(event.Apps))
			for _, app := range event.Apps {
				apps = append(apps, appSnapshot{Name: app.Name, Port: app.Port})
			}
			send(appsEvent{apps: apps})
		}
	}()
	closeFn := func() {
		_ = stream.Close()
	}
	return &appsStream{updates: updates, close: closeFn}, nil
}

func startAppsStreamCmd(state *shellState) tea.Cmd {
	if state == nil || state.appsStream != nil {
		return nil
	}
	return func() tea.Msg {
		if state.gateway == nil {
			return appsStreamStartedMsg{err: errors.New("gateway not connected")}
		}
		stream, err := startAppsStream(state.gateway)
		return appsStreamStartedMsg{stream: stream, err: err}
	}
}

func listenAppsStreamCmd(stream *appsStream) tea.Cmd {
	return func() tea.Msg {
		if stream == nil {
			return appsStreamUpdateMsg{err: errors.New("apps stream not connected")}
		}
		event, ok := <-stream.updates
		if !ok {
			return appsStreamUpdateMsg{err: errors.New("apps stream closed")}
		}
		return appsStreamUpdateMsg(event)
	}
}

func refreshAppsFromStreamCmd(state *shellState, apps []appSnapshot) tea.Cmd {
	return func() tea.Msg {
		if state == nil || state.gateway == nil {
			return appsLoadedMsg{err: errors.New("gateway not connected")}
		}
		proxyEnabled := false
		if summary, err := fetchRemoteProxyConfig(state.gateway); err == nil {
			proxyEnabled = summary.Enabled
		}
		summaries, err := buildAppSummaries(state.gateway, apps, proxyEnabled)
		return appsLoadedMsg{apps: summaries, err: err}
	}
}
