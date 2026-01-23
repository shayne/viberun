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

	"github.com/shayne/viberun/internal/agents"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
)

type preparedSession struct {
	resolved   target.Resolved
	gateway    *gatewayClient
	stream     *mux.Stream
	cleanup    func()
	outputTail *tailBuffer
}

func runShellAction(state *shellState, action shellAction) error {
	switch action.kind {
	case actionVibe:
		return runShellInteractive(state, action.app, "")
	case actionShell:
		return runShellInteractive(state, action.app, "shell")
	case actionDelete:
		return runShellDelete(state, action.app)
	case actionProxySetup:
		return runShellProxySetup(state, action.host)
	case actionUsersAdd:
		return runShellUsersAdd(state, action.username, action.host)
	case actionUsersRemove:
		return runShellUsersRemove(state, action.username, action.host)
	case actionUsersSetPassword:
		return runShellUsersSetPassword(state, action.username, action.host)
	case actionUsersEditor:
		return runShellUsersEditor(state, action.app)
	case actionWipe:
		return runShellWipe(state, action.host)
	default:
		return nil
	}
}

func runPreparedInteractive(state *shellState, session *preparedSession) error {
	if session == nil {
		return errors.New("missing prepared session")
	}
	if session.cleanup != nil {
		defer session.cleanup()
	}
	if err := runInteractiveMuxSession(session.resolved, session.gateway, session.stream, session.outputTail); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			maybeClearDefaultAgentOnFailure(state.cfg, state.cfgPath, runFlags{}, session.resolved.App, session.outputTail.String())
			return newSilentError(exitErr)
		}
		return err
	}
	return nil
}

func prepareInteractiveSession(state *shellState, appArg string, action string) (*preparedSession, bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, false, errors.New("interactive sessions require a TTY")
	}
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	resolved, err := target.Resolve(strings.TrimSpace(appArg), cfg)
	if err != nil {
		return nil, false, err
	}
	gateway, cleanup, err := gatewayForResolvedHost(state, resolved.Host)
	if err != nil {
		return nil, false, err
	}
	cleanupGateway := func() {
		if cleanup != nil {
			cleanup()
		}
	}

	agentProvider := strings.TrimSpace(state.agent)
	if agentProvider == "" {
		cleanupGateway()
		return nil, true, nil
	}
	agentProviderForChecks := agentProvider
	if agentProviderForChecks == "" {
		agentProviderForChecks = agents.DefaultProvider()
	}
	exists, err := remoteContainerExists(gateway, resolved.App, agentProviderForChecks)
	if err != nil {
		cleanupGateway()
		return nil, false, err
	}
	if !exists {
		cleanupGateway()
		return nil, true, nil
	}

	agentSpec, err := agents.Resolve(agentProvider)
	if err != nil {
		cleanupGateway()
		return nil, false, err
	}
	agentProvider = agentSpec.Provider

	sessionEnv := map[string]string{}
	if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
		sessionEnv["COLORTERM"] = colorTerm
	}
	if termValue := strings.TrimSpace(os.Getenv("TERM")); termValue != "" {
		sessionEnv["TERM"] = termValue
	}
	if agentCheck := strings.TrimSpace(os.Getenv("VIBERUN_AGENT_CHECK")); agentCheck != "" {
		sessionEnv["VIBERUN_AGENT_CHECK"] = agentCheck
	}

	cfgUser, err := discoverLocalUserConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "user config discovery failed: %v\n", err)
	} else if encoded, err := encodeUserConfig(cfgUser); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode user config: %v\n", err)
	} else if encoded != "" {
		sessionEnv["VIBERUN_USER_CONFIG"] = encoded
	}

	finalCleanup := cleanupGateway

	if err := gateway.startOpenStream(func(url string) {
		if err := openURL(url); err != nil {
			fmt.Fprintf(os.Stderr, "open url failed: %v\n", err)
		}
	}); err != nil {
		finalCleanup()
		return nil, false, err
	}

	ptyMeta := muxrpc.PtyMeta{
		App:    resolved.App,
		Action: strings.TrimSpace(action),
		Agent:  agentProvider,
		Env:    sessionEnv,
	}
	ptyStream, err := gateway.openStream("pty", ptyMeta)
	if err != nil {
		finalCleanup()
		return nil, false, err
	}

	outputTail := &tailBuffer{max: 32 * 1024}
	return &preparedSession{
		resolved:   resolved,
		gateway:    gateway,
		stream:     ptyStream,
		cleanup:    finalCleanup,
		outputTail: outputTail,
	}, false, nil
}

func runShellInteractive(state *shellState, appArg string, action string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("interactive sessions require a TTY")
	}
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	resolved, err := target.Resolve(strings.TrimSpace(appArg), cfg)
	if err != nil {
		return err
	}
	gateway, cleanup, err := gatewayForResolvedHost(state, resolved.Host)
	if err != nil {
		return err
	}
	defer cleanup()

	agentProvider := strings.TrimSpace(state.agent)
	agentProviderForChecks := agentProvider
	if agentProviderForChecks == "" {
		agentProviderForChecks = agents.DefaultProvider()
	}

	sessionEnv := map[string]string{}
	if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
		sessionEnv["COLORTERM"] = colorTerm
	}
	if termValue := strings.TrimSpace(os.Getenv("TERM")); termValue != "" {
		sessionEnv["TERM"] = termValue
	}
	if agentCheck := strings.TrimSpace(os.Getenv("VIBERUN_AGENT_CHECK")); agentCheck != "" {
		sessionEnv["VIBERUN_AGENT_CHECK"] = agentCheck
	}

	needsCreate := false
	exists, err := remoteContainerExists(gateway, resolved.App, agentProviderForChecks)
	if err != nil {
		return err
	}
	if !exists {
		if !promptCreateLocal(resolved.App) {
			fmt.Fprintln(os.Stderr, "aborted")
			return newSilentError(errors.New("aborted"))
		}
		needsCreate = true
		sessionEnv["VIBERUN_AUTO_CREATE"] = "1"
	}

	if agentProvider == "" {
		selection, err := tui.SelectDefaultAgent(os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if strings.TrimSpace(selection) != "" {
			cfg.AgentProvider = selection
			if err := config.Save(state.cfgPath, cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			state.cfg = cfg
			state.agent = selection
			agentProvider = selection
		}
	}
	if strings.TrimSpace(agentProvider) == "" {
		agentProvider = agentProviderForChecks
	}
	agentSpec, err := agents.Resolve(agentProvider)
	if err != nil {
		return err
	}
	agentProvider = agentSpec.Provider

	if needsCreate {
		localAuth, details, err := discoverLocalAuth(agentSpec.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth discovery failed: %v\n", err)
		} else if localAuth != nil && promptCopyAuth(resolved.App, agentSpec.Label, details) {
			bundle, err := stageAuthBundle(gateway, localAuth)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to stage auth: %v\n", err)
			} else if encoded, err := encodeAuthBundle(bundle); err != nil {
				fmt.Fprintf(os.Stderr, "failed to encode auth: %v\n", err)
			} else if encoded != "" {
				sessionEnv["VIBERUN_AUTH_BUNDLE"] = encoded
			}
		}
	}
	cfgUser, err := discoverLocalUserConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "user config discovery failed: %v\n", err)
	} else if encoded, err := encodeUserConfig(cfgUser); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode user config: %v\n", err)
	} else if encoded != "" {
		sessionEnv["VIBERUN_USER_CONFIG"] = encoded
	}

	if err := gateway.startOpenStream(func(url string) {
		if err := openURL(url); err != nil {
			fmt.Fprintf(os.Stderr, "open url failed: %v\n", err)
		}
	}); err != nil {
		return err
	}
	ptyMeta := muxrpc.PtyMeta{
		App:    resolved.App,
		Action: strings.TrimSpace(action),
		Agent:  agentProvider,
		Env:    sessionEnv,
	}
	ptyStream, err := gateway.openStream("pty", ptyMeta)
	if err != nil {
		return err
	}
	var outputTail tailBuffer
	outputTail.max = 32 * 1024
	if err := runInteractiveMuxSession(resolved, gateway, ptyStream, &outputTail); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			maybeClearDefaultAgentOnFailure(cfg, state.cfgPath, runFlags{}, resolved.App, outputTail.String())
			return newSilentError(exitErr)
		}
		return err
	}
	return nil
}

func runShellDelete(state *shellState, appArg string) error {
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	resolved, err := target.Resolve(strings.TrimSpace(appArg), cfg)
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("delete requires a TTY")
	}
	if !promptDelete(resolved.App) {
		state.appendOutput("delete cancelled")
		return nil
	}
	gateway, cleanup, err := gatewayForResolvedHost(state, resolved.Host)
	if err != nil {
		return err
	}
	defer cleanup()
	remoteArgs := buildAppCommandArgs(strings.TrimSpace(state.agent), resolved.App, []string{"delete"})
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return err
	}
	appendCommandOutput(state, output)
	return nil
}

func runShellProxySetup(state *shellState, hostArg string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("proxy setup requires a TTY")
	}
	gateway, cleanup, err := ensureShellGatewayForHost(state, hostArg)
	if err != nil {
		return err
	}
	defer cleanup()
	resolved, err := resolveShellHost(state, hostArg)
	if err != nil {
		return err
	}
	if err := runProxySetupFlow(resolved.Host, gateway, proxySetupOptions{updateArtifacts: true, forceSetup: true}); err != nil {
		return err
	}
	return nil
}

func runShellUsersAdd(state *shellState, username string, hostArg string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("user management requires a TTY")
	}
	password, err := tui.PromptPassword(os.Stdin, os.Stdout, "Password")
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	gateway, cleanup, err := ensureShellGatewayForHost(state, hostArg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runRemoteUsersAdd(gateway, username, password); err != nil {
		return err
	}
	state.appendOutput("OK")
	return nil
}

func runShellUsersRemove(state *shellState, username string, hostArg string) error {
	gateway, cleanup, err := ensureShellGatewayForHost(state, hostArg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runRemoteUsersRemove(gateway, username); err != nil {
		return err
	}
	state.appendOutput("OK")
	return nil
}

func runShellUsersSetPassword(state *shellState, username string, hostArg string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("user management requires a TTY")
	}
	password, err := tui.PromptPassword(os.Stdin, os.Stdout, "Password")
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	gateway, cleanup, err := ensureShellGatewayForHost(state, hostArg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runRemoteUsersSetPassword(gateway, username, password); err != nil {
		return err
	}
	state.appendOutput("OK")
	return nil
}

func runShellUsersEditor(state *shellState, appArg string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("app users requires a TTY")
	}
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	resolved, err := target.Resolve(strings.TrimSpace(appArg), cfg)
	if err != nil {
		return err
	}
	gateway, cleanup, err := gatewayForResolvedHost(state, resolved.Host)
	if err != nil {
		return err
	}
	defer cleanup()
	info, err := fetchProxyInfo(gateway, resolved.App)
	if err != nil {
		return err
	}
	updated, err := runUsersEditor(gateway, resolved.App, info)
	if err != nil {
		return err
	}
	if updated {
		state.appendOutput("Updated app user access.")
	}
	return nil
}

func runShellWipe(state *shellState, hostArg string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("wipe requires a TTY")
	}
	resolved, err := resolveShellHost(state, hostArg)
	if err != nil {
		return err
	}
	confirm, err := tui.PromptWipeDecision(os.Stdin, os.Stdout, resolved.Host)
	if err != nil {
		return err
	}
	if !confirm {
		state.appendOutput("Wipe cancelled.")
		return nil
	}
	if err := tui.PromptWipeConfirm(os.Stdin, os.Stdout); err != nil {
		return err
	}
	gateway, cleanup, err := ensureShellGatewayForHost(state, hostArg)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := runRemoteWipe(gateway); err != nil {
		return err
	}
	if err := config.RemoveConfigFiles(); err != nil {
		return err
	}
	state.appendOutput("Wipe complete.")
	return nil
}

func ensureShellGatewayForHost(state *shellState, hostArg string) (*gatewayClient, func(), error) {
	resolved, err := resolveShellHost(state, hostArg)
	if err != nil {
		return nil, func() {}, err
	}
	sameHost := false
	if strings.TrimSpace(hostArg) == "" {
		sameHost = true
	} else if resolvedState, err := resolveShellHost(state, ""); err == nil && resolvedState.Host == resolved.Host {
		sameHost = true
	}
	if sameHost && state.gateway != nil && state.gatewayHost == resolved.Host {
		return state.gateway, func() {}, nil
	}
	gateway, err := startGateway(resolved.Host, strings.TrimSpace(state.agent), nil, false)
	if err != nil {
		return nil, func() {}, err
	}
	if sameHost {
		closeShellGateway(state)
		state.gateway = gateway
		state.gatewayHost = resolved.Host
		return gateway, func() {}, nil
	}
	cleanup := func() { _ = gateway.Close() }
	return gateway, cleanup, nil
}

func gatewayForResolvedHost(state *shellState, host string) (*gatewayClient, func(), error) {
	if strings.TrimSpace(host) == "" {
		return nil, func() {}, errors.New("host is required")
	}
	resolvedState, err := resolveShellHost(state, "")
	sameHost := err == nil && resolvedState.Host == host
	if sameHost && state.gateway != nil && state.gatewayHost == host {
		return state.gateway, func() {}, nil
	}
	gateway, err := startGateway(host, strings.TrimSpace(state.agent), nil, false)
	if err != nil {
		return nil, func() {}, err
	}
	if sameHost {
		closeShellGateway(state)
		state.gateway = gateway
		state.gatewayHost = host
		return gateway, func() {}, nil
	}
	cleanup := func() { _ = gateway.Close() }
	return gateway, cleanup, nil
}

func appendCommandOutput(state *shellState, output string) {
	if state == nil {
		return
	}
	trimmed := strings.TrimRight(output, "\n")
	if strings.TrimSpace(trimmed) == "" {
		state.appendOutput("OK")
		return
	}
	state.appendOutput(trimmed)
}
