// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"sync"
	"time"
)

type forwardManager struct {
	gateway     *gatewayClient
	gatewayHost string

	mu       sync.Mutex
	forwards map[string]appForward

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func startForwardManager(state *shellState) {
	if state == nil || state.gateway == nil {
		return
	}
	host := strings.TrimSpace(state.gatewayHost)
	if state.forwarder != nil && state.forwarder.matches(state.gateway, host) {
		return
	}
	stopForwardManager(state)
	manager := &forwardManager{
		gateway:     state.gateway,
		gatewayHost: host,
		forwards:    map[string]appForward{},
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	state.forwarder = manager
	go manager.run()
}

func stopForwardManager(state *shellState) {
	if state == nil || state.forwarder == nil {
		return
	}
	state.forwarder.Stop()
	state.forwarder = nil
}

func (m *forwardManager) matches(gateway *gatewayClient, host string) bool {
	if m == nil {
		return false
	}
	return m.gateway == gateway && strings.TrimSpace(m.gatewayHost) == strings.TrimSpace(host)
}

func (m *forwardManager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stopCh) })
	<-m.doneCh
}

func (m *forwardManager) run() {
	if m == nil {
		return
	}
	defer close(m.doneCh)
	if m.gateway == nil {
		return
	}
	for {
		if m.isStopped() {
			m.closeAllForwards()
			return
		}
		stream, err := startAppsStream(m.gateway)
		if err != nil {
			if !m.sleepOrStop(2 * time.Second) {
				m.closeAllForwards()
				return
			}
			continue
		}
		if !m.consumeStream(stream) {
			m.closeAllForwards()
			return
		}
		if !m.sleepOrStop(1 * time.Second) {
			m.closeAllForwards()
			return
		}
	}
}

func (m *forwardManager) consumeStream(stream *appsStream) bool {
	if stream == nil {
		return m.sleepOrStop(2 * time.Second)
	}
	defer func() {
		if stream.close != nil {
			stream.close()
		}
	}()
	for {
		select {
		case <-m.stopCh:
			return false
		case event, ok := <-stream.updates:
			if !ok {
				return true
			}
			if event.err != nil {
				return true
			}
			m.syncFromSnapshots(event.apps)
		}
	}
}

func (m *forwardManager) sleepOrStop(duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-m.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func (m *forwardManager) isStopped() bool {
	select {
	case <-m.stopCh:
		return true
	default:
		return false
	}
}

func (m *forwardManager) closeAllForwards() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, forward := range m.forwards {
		if forward.close != nil {
			forward.close()
		}
		delete(m.forwards, name)
	}
}

func (m *forwardManager) syncFromSummaries(summaries []appSummary) {
	apps := make([]appSnapshot, 0, len(summaries))
	for _, summary := range summaries {
		name := strings.TrimSpace(summary.Name)
		if name == "" {
			continue
		}
		apps = append(apps, appSnapshot{Name: name, Port: summary.Port})
	}
	m.syncFromSnapshots(apps)
}

func (m *forwardManager) syncFromSnapshots(apps []appSnapshot) {
	if m == nil || m.gateway == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if isLocalHost(m.gatewayHost) {
		for name, forward := range m.forwards {
			if forward.close != nil {
				forward.close()
			}
			delete(m.forwards, name)
		}
		return
	}

	keep := map[string]int{}
	for _, app := range apps {
		name := strings.TrimSpace(app.Name)
		if name == "" {
			continue
		}
		keep[name] = app.Port
	}
	for name, forward := range m.forwards {
		if _, ok := keep[name]; ok {
			continue
		}
		if forward.close != nil {
			forward.close()
		}
		delete(m.forwards, name)
	}

	for _, app := range apps {
		name := strings.TrimSpace(app.Name)
		if name == "" {
			continue
		}
		port := app.Port
		if port <= 0 {
			delete(m.forwards, name)
			continue
		}
		if forward, ok := m.forwards[name]; ok && forward.port == port && forward.err == nil {
			continue
		}
		if forward, ok := m.forwards[name]; ok && forward.close != nil {
			forward.close()
		}
		if err := ensureLocalPortAvailable(port); err != nil {
			m.forwards[name] = appForward{port: port, err: err}
			continue
		}
		closeFn, err := startLocalForwardMux(m.gateway, port)
		if err != nil {
			m.forwards[name] = appForward{port: port, err: err}
			continue
		}
		m.forwards[name] = appForward{port: port, close: closeFn}
	}
}

func (m *forwardManager) status(name string, port int) (bool, error, bool) {
	if m == nil {
		return false, nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	forward, ok := m.forwards[name]
	if !ok || forward.port != port {
		return false, nil, false
	}
	if forward.err != nil {
		return false, forward.err, true
	}
	return true, nil, true
}
