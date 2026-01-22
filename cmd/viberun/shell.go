// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/target"
)

type shellScope int

const (
	scopeGlobal shellScope = iota
	scopeAppConfig
)

type connectionState int

const (
	connUnknown connectionState = iota
	connConnecting
	connConnected
	connFailed
)

type externalCommand struct {
	args []string
}

type setupAction struct {
	host string
}

type shellState struct {
	output       []string
	history      []string
	historyIdx   int
	scope        shellScope
	app          string
	prevApp      string
	apps         []string
	appsLoaded   bool
	syncing      bool
	pendingCmd   *pendingCommand
	host         string
	agent        string
	cfg          config.Config
	cfgPath      string
	connState    connectionState
	connError    string
	bootstrapped bool
	setupNeeded  bool
	setupHinted  bool
	hostPrompt   bool
	setupAction  *setupAction
	externalCmd  *externalCommand
	quit         bool
	devMode      bool
}

func runShell() error {
	state, err := newShellState()
	if err != nil {
		return err
	}
	for {
		model := newShellModel(state)
		program := tea.NewProgram(model)
		finalModel, err := program.Run()
		if err != nil {
			return err
		}
		m, ok := finalModel.(shellModel)
		if ok {
			state = m.state
		}
		if state.quit {
			return nil
		}
		if state.setupAction != nil {
			action := *state.setupAction
			state.setupAction = nil
			ran, note, err := runShellSetup(state, action)
			if err != nil {
				var quietErr silentError
				if !errors.As(err, &quietErr) {
					state.appendOutput(fmt.Sprintf("setup failed: %v", err))
				}
			} else if ran {
				state.setupNeeded = false
				state.setupHinted = false
				state.hostPrompt = false
				state.bootstrapped = true
				state.output = nil
				state.appendOutput(renderFirstAppHint())
			}
			if note != "" {
				state.appendOutput(note)
			}
			state.connState = connConnecting
			continue
		}
		if state.externalCmd != nil {
			if err := runExternalCommand(*state.externalCmd); err != nil {
				state.appendOutput(fmt.Sprintf("error: %v", err))
			}
			state.externalCmd = nil
			state.connState = connConnecting
			continue
		}
		return nil
	}
}

func newShellState() (*shellState, error) {
	cfg, path, err := config.Load()
	if err != nil {
		return nil, err
	}
	state := &shellState{
		output:       []string{},
		history:      []string{},
		historyIdx:   -1,
		scope:        scopeGlobal,
		appsLoaded:   false,
		cfg:          cfg,
		cfgPath:      path,
		connState:    connUnknown,
		devMode:      isDevMode(),
		bootstrapped: false,
	}
	state.host = strings.TrimSpace(cfg.DefaultHost)
	state.agent = strings.TrimSpace(cfg.AgentProvider)
	if state.host == "" {
		state.hostPrompt = true
		state.setupNeeded = true
	}
	return state, nil
}

func (s *shellState) resolvedHost() (target.ResolvedHost, error) {
	return target.ResolveHost(s.host, s.cfg)
}

func (s *shellState) appendOutput(text string) {
	if text == "" {
		s.output = append(s.output, "")
		return
	}
	trimmed := strings.TrimRight(text, "\n")
	trailing := len(text) - len(trimmed)
	if trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		s.output = append(s.output, lines...)
	}
	for i := 0; i < trailing; i++ {
		s.output = append(s.output, "")
	}
}

func shouldStartShell() bool {
	if os.Getenv("VIBERUN_NO_SHELL") != "" {
		return false
	}
	if len(os.Args) > 1 {
		return false
	}
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	return stdinTTY && stdoutTTY
}

func runExternalCommand(cmd externalCommand) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if len(cmd.args) == 0 {
		return errors.New("missing external command args")
	}
	command := exec.Command(exe, cmd.args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	command.Env = os.Environ()
	return command.Run()
}

func isDevMode() bool {
	if strings.TrimSpace(os.Getenv("VIBERUN_DEV")) != "" {
		return true
	}
	return isDevRun() || isDevVersion()
}
