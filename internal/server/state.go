// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const basePort = 8080

// State tracks persisted server allocations.
type State struct {
	Ports map[string]int `json:"ports"`
}

func LoadState() (State, string, error) {
	path, err := statePath()
	if err != nil {
		return State{}, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Ports: map[string]int{}}, path, nil
		}
		return State{}, path, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, path, err
	}
	if state.Ports == nil {
		state.Ports = map[string]int{}
	}

	return state, path, nil
}

func SaveState(path string, state State) error {
	if state.Ports == nil {
		state.Ports = map[string]int{}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func (s *State) PortForApp(app string) (int, bool) {
	if s.Ports == nil {
		return 0, false
	}
	port, ok := s.Ports[app]
	return port, ok
}

func (s *State) AssignPort(app string) int {
	if s.Ports == nil {
		s.Ports = map[string]int{}
	}
	if port, ok := s.Ports[app]; ok {
		return port
	}

	used := make(map[int]bool, len(s.Ports))
	for _, port := range s.Ports {
		used[port] = true
	}

	port := basePort
	for used[port] {
		port++
	}

	s.Ports[app] = port
	return port
}

func (s *State) SetPort(app string, port int) {
	if s.Ports == nil {
		s.Ports = map[string]int{}
	}
	s.Ports[app] = port
}

func (s *State) RemoveApp(app string) bool {
	if s.Ports == nil {
		return false
	}
	if _, ok := s.Ports[app]; !ok {
		return false
	}
	delete(s.Ports, app)
	return true
}

func statePath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		var err error
		configHome, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(configHome, "viberun", "server-state.json"), nil
}
