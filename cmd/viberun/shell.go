// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	tea "charm.land/bubbletea/v2"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/tui/dialogs"
	"github.com/shayne/viberun/internal/tui/theme"
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

type setupAction struct {
	host string
	plan *setupPlan
}

type appStatus string

const (
	appStatusRunning appStatus = "running"
	appStatusStopped appStatus = "stopped"
	appStatusUnknown appStatus = "unknown"
)

type appSummary struct {
	Name       string
	Status     appStatus
	LocalURL   string
	PublicURL  string
	Port       int
	Forwarded  bool
	ForwardErr string
}

type appForward struct {
	port  int
	close func()
	err   error
}

type shellActionKind int

const (
	actionVibe shellActionKind = iota
	actionShell
	actionDelete
	actionProxySetup
	actionUsersAdd
	actionUsersRemove
	actionUsersSetPassword
	actionUsersEditor
	actionWipe
)

type shellAction struct {
	kind         shellActionKind
	app          string
	host         string
	username     string
	proxyPlan    *proxyPlan
	wipePlan     *wipePlan
	passwordPlan *passwordPlan
}

type shellState struct {
	output             []string
	history            []string
	historyIdx         int
	scope              shellScope
	app                string
	prevApp            string
	apps               []appSummary
	appsLoaded         bool
	appsSyncing        bool
	appsRenderPending  bool
	preparedSession    *preparedSession
	syncing            bool
	pendingCmd         *pendingCommand
	host               string
	agent              string
	cfg                config.Config
	cfgPath            string
	connState          connectionState
	connError          string
	bootstrapped       bool
	setupNeeded        bool
	setupHinted        bool
	hostPrompt         bool
	startupActive      bool
	startupStage       string
	startupRendered    bool
	startupOutputStart int
	startupOutputEnd   int
	headerRendered     bool
	clearOnStart       bool
	resumeAfterAction  bool
	confirmDeleteApp   string
	setupAction        *setupAction
	shellAction        *shellAction
	promptFlow         promptFlow
	promptDialog       dialogs.Dialog
	quit               bool
	devMode            bool
	gateway            *gatewayClient
	gatewayHost        string
	appForwards        map[string]appForward
	forwarder          *forwardManager
	appsStream         *appsStream
}

func runShell() error {
	state, err := newShellState()
	if err != nil {
		return err
	}
	defer func() {
		closeShellGateway(state)
	}()
	for {
		model := newShellModel(state)
		// Keep raw mode enabled by wrapping stdin with a term.File-compatible filter.
		input := newOSCFilteringTTY(os.Stdin)
		theme.SetOSCReadHook(input.WaitOSC)
		program := tea.NewProgram(model, tea.WithInput(input))
		finalModel, err := program.Run()
		if err != nil {
			return err
		}
		if m, ok := finalModel.(shellModel); ok {
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
		if state.preparedSession != nil {
			session := state.preparedSession
			state.preparedSession = nil
			if err := runPreparedInteractive(state, session); err != nil {
				state.appendOutput(fmt.Sprintf("error: %v", err))
			}
			flushTerminalInputBuffer()
			state.clearOnStart = true
			state.resumeAfterAction = true
			continue
		}
		if state.shellAction != nil {
			action := *state.shellAction
			state.shellAction = nil
			if err := runShellAction(state, action); err != nil {
				state.appendOutput(fmt.Sprintf("error: %v", err))
			}
			flushTerminalInputBuffer()
			state.clearOnStart = actionClearsTerminal(action.kind)
			if actionResumesShell(action.kind) {
				state.resumeAfterAction = true
			} else {
				state.startupRendered = true
				state.connState = connConnecting
			}
			continue
		}
		return nil
	}
}

func actionClearsTerminal(kind shellActionKind) bool {
	switch kind {
	case actionVibe, actionShell:
		return true
	default:
		return false
	}
}

func actionResumesShell(kind shellActionKind) bool {
	switch kind {
	case actionVibe, actionShell, actionDelete:
		return true
	default:
		return false
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
		appForwards:  map[string]appForward{},
	}
	state.host = strings.TrimSpace(cfg.DefaultHost)
	state.agent = strings.TrimSpace(cfg.AgentProvider)
	if state.host == "" {
		state.hostPrompt = true
		state.setupNeeded = true
	}
	return state, nil
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

func (s *shellState) appendStartupOutput(text string) {
	if s == nil {
		return
	}
	s.startupOutputStart = len(s.output)
	s.appendOutput(text)
	s.startupOutputEnd = len(s.output)
}

func confirmDeletePrompt(state *shellState) string {
	if state == nil || strings.TrimSpace(state.confirmDeleteApp) == "" {
		return ""
	}
	return fmt.Sprintf("Delete %s and all snapshots? [y/N]: ", state.confirmDeleteApp)
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

func isDevMode() bool {
	return isLocalDevMode()
}
