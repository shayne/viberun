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
	"github.com/shayne/viberun/internal/muxrpc"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
)

type preparedSession struct {
	resolved   target.Resolved
	gateway    *gatewayClient
	ptyMeta    muxrpc.PtyMeta
	cleanup    func()
	outputTail *tailBuffer
}

func runShellAction(state *shellState, action shellAction) error {
	switch action.kind {
	case actionVibe:
		return runShellAttachSubprocess(state, action.app, "")
	case actionShell:
		return runShellAttachSubprocess(state, action.app, "shell")
	case actionDelete:
		return runShellDelete(state, action.app)
	case actionProxySetup:
		return runShellProxySetup(state, action.host, action.proxyPlan)
	case actionUsersAdd:
		return runShellUsersAdd(state, action.username, action.host, action.passwordPlan)
	case actionUsersRemove:
		return runShellUsersRemove(state, action.username, action.host)
	case actionUsersSetPassword:
		return runShellUsersSetPassword(state, action.username, action.host, action.passwordPlan)
	case actionUsersEditor:
		return runShellUsersEditor(state, action.app)
	case actionWipe:
		return runShellWipe(state, action.host, action.wipePlan)
	default:
		return nil
	}
}

func runShellAttachSubprocess(state *shellState, app string, action string) error {
	args, err := buildAttachArgs(state, app, action)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	flushTerminalInputBuffer()
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func buildAttachArgs(state *shellState, app string, action string) ([]string, error) {
	if strings.TrimSpace(app) == "" {
		return nil, errors.New("app is required")
	}
	args := []string{"attach"}
	if strings.TrimSpace(action) == "shell" {
		args = append(args, "--shell")
	}
	if state != nil {
		if host := strings.TrimSpace(state.host); host != "" {
			args = append(args, "--host", host)
		}
		if agent := strings.TrimSpace(state.agent); agent != "" {
			args = append(args, "--agent", agent)
		}
	}
	args = append(args, app)
	return args, nil
}

func runPreparedInteractive(state *shellState, session *preparedSession) error {
	if session == nil {
		return errors.New("missing prepared session")
	}
	if session.cleanup != nil {
		defer session.cleanup()
	}
	action := strings.TrimSpace(session.ptyMeta.Action)
	if err := runShellAttachSubprocess(state, session.resolved.App, action); err != nil {
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
	gateway, cleanup, err := startAttachGateway(state, resolved.Host)
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
	for key, value := range sessionTermEnv() {
		sessionEnv[key] = value
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

	ptyMeta := muxrpc.PtyMeta{
		App:    resolved.App,
		Action: strings.TrimSpace(action),
		Agent:  agentProvider,
		Env:    sessionEnv,
	}

	outputTail := &tailBuffer{max: 32 * 1024}
	return &preparedSession{
		resolved:   resolved,
		gateway:    gateway,
		ptyMeta:    ptyMeta,
		cleanup:    cleanupGateway,
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
	gateway, cleanup, err := startAttachGateway(state, resolved.Host)
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
	for key, value := range sessionTermEnv() {
		sessionEnv[key] = value
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

	ptyMeta := muxrpc.PtyMeta{
		App:    resolved.App,
		Action: strings.TrimSpace(action),
		Agent:  agentProvider,
		Env:    sessionEnv,
	}
	var outputTail tailBuffer
	outputTail.max = 32 * 1024
	attach := AttachSession{
		Resolved:   resolved,
		Gateway:    gateway,
		PtyMeta:    ptyMeta,
		OutputTail: &outputTail,
		OpenURL:    openURL,
	}
	if err := attach.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			maybeClearDefaultAgentOnFailure(cfg, state.cfgPath, "", resolved.App, outputTail.String())
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

func runDeleteConfirmed(state *shellState, appArg string) (string, error) {
	if state == nil {
		return "", errors.New("missing shell state")
	}
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	resolved, err := target.Resolve(strings.TrimSpace(appArg), cfg)
	if err != nil {
		return "", err
	}
	gateway, cleanup, err := gatewayForResolvedHost(state, resolved.Host)
	if err != nil {
		return "", err
	}
	defer cleanup()
	remoteArgs := buildAppCommandArgs(strings.TrimSpace(state.agent), resolved.App, []string{"delete"})
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return "", err
	}
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = fmt.Sprintf("Deleted app %s", resolved.App)
	}
	return output, nil
}

func runShellProxySetup(state *shellState, hostArg string, plan *proxyPlan) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("proxy setup requires a TTY")
	}
	if plan != nil && strings.TrimSpace(plan.Host) != "" {
		hostArg = plan.Host
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
	if err := runProxySetupFlow(resolved.Host, gateway, proxySetupOptions{updateArtifacts: true, forceSetup: true}, plan); err != nil {
		return err
	}
	return nil
}

func runShellUsersAdd(state *shellState, username string, hostArg string, plan *passwordPlan) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("user management requires a TTY")
	}
	password := ""
	if plan != nil {
		password = plan.Password
	}
	if strings.TrimSpace(password) == "" {
		var err error
		password, err = tui.PromptPassword(os.Stdin, os.Stdout, "Password")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
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

func runShellUsersSetPassword(state *shellState, username string, hostArg string, plan *passwordPlan) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("user management requires a TTY")
	}
	password := ""
	if plan != nil {
		password = plan.Password
	}
	if strings.TrimSpace(password) == "" {
		var err error
		password, err = tui.PromptPassword(os.Stdin, os.Stdout, "Password")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
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

func runShellWipe(state *shellState, hostArg string, plan *wipePlan) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("wipe requires a TTY")
	}
	resolved, err := resolveShellHost(state, hostArg)
	if err != nil {
		return err
	}
	activePlan := plan
	if activePlan == nil {
		activePlan = &wipePlan{Host: resolved.Host, WipeLocal: true}
	}
	wiped, err := runWipeFlow(resolved.Host, activePlan.WipeLocal, activePlan)
	if err != nil {
		return err
	}
	if !wiped {
		state.appendOutput("Wipe cancelled.")
		return nil
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
	gateway, err := startGateway(resolved.Host, strings.TrimSpace(state.agent), devChannelEnv(), false)
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
	gateway, err := startGateway(host, strings.TrimSpace(state.agent), devChannelEnv(), false)
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

func startAttachGateway(state *shellState, host string) (*gatewayClient, func(), error) {
	if strings.TrimSpace(host) == "" {
		return nil, func() {}, errors.New("host is required")
	}
	gateway, err := startGateway(host, strings.TrimSpace(state.agent), devChannelEnv(), false)
	if err != nil {
		return nil, func() {}, err
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

func sessionTermEnv() map[string]string {
	env := map[string]string{}
	termValue := strings.TrimSpace(os.Getenv("TERM"))
	if termValue == "" {
		termValue = "xterm-256color"
	}
	switch strings.ToLower(termValue) {
	case "xterm-ghostty", "ghostty", "xterm-kitty", "kitty", "wezterm", "alacritty", "foot", "dumb":
		termValue = "xterm-256color"
	}
	env["TERM"] = termValue
	if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
		env["COLORTERM"] = colorTerm
	}
	return env
}
