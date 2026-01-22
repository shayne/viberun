// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pelletier/go-toml/v2"

	"github.com/shayne/viberun/internal/agents"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/target"
)

type parsedCommand struct {
	name            string
	args            []string
	enforceExisting bool
}

type pendingCommand struct {
	cmd   parsedCommand
	scope shellScope
}

type helpLine struct {
	cmd    string
	desc   string
	indent bool
}

func dispatchShellCommand(state *shellState, line string) (string, tea.Cmd) {
	cmd, err := parseShellCommand(line)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	return executeShellCommand(state, cmd, state.scope, true)
}

func setAppContext(state *shellState, app string) {
	state.prevApp = state.app
	if strings.TrimSpace(app) == "" {
		state.scope = scopeGlobal
		state.app = ""
		return
	}
	state.scope = scopeAppConfig
	state.app = app
}

func normalizeCommand(cmd parsedCommand) parsedCommand {
	switch cmd.name {
	case "ls":
		cmd.name = "apps"
		return cmd
	case "cd..":
		return parsedCommand{name: "cd", args: []string{".."}}
	case "cd-":
		return parsedCommand{name: "cd", args: []string{"-"}}
	default:
		if strings.HasPrefix(cmd.name, "./") {
			app := strings.TrimPrefix(cmd.name, "./")
			if app == "" {
				return parsedCommand{name: "run", args: cmd.args}
			}
			return parsedCommand{
				name:            "run",
				args:            append([]string{app}, cmd.args...),
				enforceExisting: true,
			}
		}
	}
	return cmd
}

func executeShellCommand(state *shellState, cmd parsedCommand, scope shellScope, allowDefer bool) (string, tea.Cmd) {
	cmd = normalizeCommand(cmd)
	if cmd.name == "" {
		return "", nil
	}
	if cmd.name == "cd" {
		return handleCDCommand(state, cmd, scope, allowDefer)
	}
	if requiresHostSync(scope, cmd) {
		if state.hostPrompt {
			return renderShellError("error: no server connected yet. Run `setup` to get started."), nil
		}
		if state.setupNeeded {
			return renderShellError("error: server not set up yet. Run `setup` to finish setup."), nil
		}
		needsGateway := requiresGatewayForCommand(cmd)
		needsApps := requiresAppsListForCommand(cmd, scope)
		if allowDefer && (state.syncing || (needsApps && !state.appsLoaded) || (needsGateway && !gatewayReady(state))) {
			if msg, ok := localAppValidationError(state, cmd); ok {
				return msg, nil
			}
			state.pendingCmd = &pendingCommand{cmd: cmd, scope: scopeGlobal}
			if needsGateway && !gatewayReady(state) {
				return "", startHostSync(state)
			}
			if needsApps && !state.appsLoaded {
				if state.syncing {
					return "", waitCmd()
				}
				if !gatewayReady(state) {
					return "", startHostSync(state)
				}
				if !state.appsSyncing {
					state.appsSyncing = true
					return "", loadAppsCmd(state, false)
				}
				return "", waitCmd()
			}
			return "", startHostSync(state)
		}
	}
	if cmd.name == "apps" || cmd.name == "ls" {
		if state.syncing || !gatewayReady(state) {
			state.pendingCmd = &pendingCommand{cmd: cmd, scope: scopeGlobal}
			return "", startHostSync(state)
		}
	}
	if scope == scopeAppConfig {
		return dispatchAppCommand(state, cmd)
	}
	return dispatchGlobalCommand(state, cmd)
}

func handleCDCommand(state *shellState, cmd parsedCommand, scope shellScope, allowDefer bool) (string, tea.Cmd) {
	if len(cmd.args) == 0 {
		return "error: cd requires a target", nil
	}
	target := strings.TrimSpace(cmd.args[0])
	switch target {
	case "..":
		setAppContext(state, "")
		return "", nil
	case "-":
		if state.prevApp == "" && state.app == "" {
			return "error: no previous app", nil
		}
		setAppContext(state, state.prevApp)
		return "", nil
	default:
		if state.hostPrompt {
			return renderShellError("error: no server connected yet. Run `setup` to get started."), nil
		}
		if state.setupNeeded {
			return renderShellError("error: server not set up yet. Run `setup` to finish setup."), nil
		}
		if allowDefer && !state.appsLoaded && !state.hostPrompt {
			state.pendingCmd = &pendingCommand{cmd: cmd, scope: scopeGlobal}
			if gatewayReady(state) {
				if !state.appsSyncing {
					state.appsSyncing = true
					return "", loadAppsCmd(state, true)
				}
				return "", waitCmd()
			}
			return "", startHostSync(state)
		}
		if !appExists(state, target) {
			return renderShellError(fmt.Sprintf("error: app %q not found", target)), nil
		}
		setAppContext(state, target)
		return "", nil
	}
}

func startHostSync(state *shellState) tea.Cmd {
	if state.syncing {
		return nil
	}
	state.syncing = true
	state.connState = connConnecting
	return syncHostCmd(state)
}

func requiresHostSync(scope shellScope, cmd parsedCommand) bool {
	// Use command registry metadata so sync gating stays consistent with new commands.
	spec, ok := lookupCommandSpec(scope, cmd.name)
	if !ok && scope == scopeAppConfig {
		spec, ok = lookupCommandSpec(scopeGlobal, cmd.name)
	}
	if !ok {
		return false
	}
	return spec.RequiresSync
}

func gatewayReady(state *shellState) bool {
	if state.gateway == nil {
		return false
	}
	if strings.TrimSpace(state.gatewayHost) == "" {
		return false
	}
	resolved, err := target.ResolveHost(state.host, state.cfg)
	if err != nil {
		return false
	}
	return resolved.Host == state.gatewayHost
}

func requiresGatewayForCommand(cmd parsedCommand) bool {
	switch cmd.name {
	case "apps", "ls", "app":
		return false
	default:
		return true
	}
}

func requiresAppsListForCommand(cmd parsedCommand, scope shellScope) bool {
	if scope == scopeAppConfig {
		return false
	}
	switch cmd.name {
	case "apps", "ls", "app", "cd":
		return true
	default:
		return cmd.enforceExisting
	}
}

func localAppValidationError(state *shellState, cmd parsedCommand) (string, bool) {
	if !state.appsLoaded || len(cmd.args) == 0 {
		return "", false
	}
	app := strings.TrimSpace(cmd.args[0])
	if app == "" || appExists(state, app) {
		return "", false
	}
	switch cmd.name {
	case "app":
		return renderShellError(fmt.Sprintf("error: app %q not found", app)), true
	case "run":
		if cmd.enforceExisting {
			return renderShellError(fmt.Sprintf("error: app %q not found. Run `run %s` to create it.", app, app)), true
		}
	case "shell", "rm", "delete":
		return renderShellError(fmt.Sprintf("error: app %q not found", app)), true
	}
	return "", false
}

func dispatchCommandWithScope(state *shellState, pending *pendingCommand) (string, tea.Cmd) {
	if pending == nil {
		return "", nil
	}
	return executeShellCommand(state, pending.cmd, pending.scope, false)
}

func appExists(state *shellState, app string) bool {
	if !state.appsLoaded {
		return false
	}
	app = strings.TrimSpace(app)
	if app == "" {
		return false
	}
	for _, name := range state.apps {
		if name == app {
			return true
		}
	}
	return false
}

func renderAppsList(state *shellState) string {
	if len(state.apps) == 0 {
		return "No apps found."
	}
	return strings.Join(state.apps, "\n")
}

func renderShellError(text string) string {
	theme := shellTheme()
	if !theme.Enabled {
		return text
	}
	return theme.Error.Render(text)
}

func dispatchGlobalCommand(state *shellState, cmd parsedCommand) (string, tea.Cmd) {
	switch cmd.name {
	case "help", "?":
		if len(cmd.args) > 0 {
			if isHelpAll(cmd.args[0]) {
				return renderGlobalHelp(true), nil
			}
			return renderCommandHelp(cmd.args[0], scopeGlobal), nil
		}
		return renderGlobalHelp(false), nil
	case "ls":
		fallthrough
	case "apps":
		if !state.appsLoaded {
			if state.appsSyncing {
				state.appsRenderPending = true
				return "", waitCmd()
			}
			if !gatewayReady(state) {
				state.pendingCmd = &pendingCommand{cmd: cmd, scope: scopeGlobal}
				return "", startHostSync(state)
			}
			state.appsSyncing = true
			return "", loadAppsCmd(state, true)
		}
		return renderAppsList(state), nil
	case "app":
		if len(cmd.args) < 1 {
			return "error: app name required", nil
		}
		if !appExists(state, cmd.args[0]) {
			return renderShellError(fmt.Sprintf("error: app %q not found", cmd.args[0])), nil
		}
		setAppContext(state, cmd.args[0])
		return fmt.Sprintf("Entered config for %s", state.app), nil
	case "run":
		if len(cmd.args) < 1 {
			return "error: run requires an app name", nil
		}
		if cmd.enforceExisting && !appExists(state, cmd.args[0]) {
			return renderShellError(fmt.Sprintf("error: app %q not found. Run `run %s` to create it.", cmd.args[0], cmd.args[0])), nil
		}
		if state.appsLoaded && !cmd.enforceExisting && !appExists(state, cmd.args[0]) {
			state.apps = append(state.apps, cmd.args[0])
		}
		return "", prepareInteractiveCmd(state, shellAction{kind: actionRun, app: cmd.args[0]})
	case "shell":
		if len(cmd.args) < 1 {
			return "error: shell requires an app name", nil
		}
		if !appExists(state, cmd.args[0]) {
			return renderShellError(fmt.Sprintf("error: app %q not found", cmd.args[0])), nil
		}
		return "", prepareInteractiveCmd(state, shellAction{kind: actionShell, app: cmd.args[0]})
	case "rm", "delete":
		if len(cmd.args) < 1 {
			return "error: rm requires an app name", nil
		}
		if !appExists(state, cmd.args[0]) {
			return renderShellError(fmt.Sprintf("error: app %q not found", cmd.args[0])), nil
		}
		return "", shellActionCmd(shellAction{kind: actionDelete, app: cmd.args[0]})
	case "config":
		return handleConfigShell(state, cmd.args)
	case "setup":
		return handleSetupShell(state, cmd.args)
	case "proxy":
		if len(cmd.args) > 0 && cmd.args[0] == "setup" {
			host := ""
			if len(cmd.args) > 1 {
				host = cmd.args[1]
			}
			return "", shellActionCmd(shellAction{kind: actionProxySetup, host: host})
		}
		return "error: usage: proxy setup [host]", nil
	case "users":
		return handleUsersShell(state, cmd.args)
	case "wipe":
		host := ""
		if len(cmd.args) > 0 {
			host = cmd.args[0]
		}
		return "", shellActionCmd(shellAction{kind: actionWipe, host: host})
	case "exit", "quit":
		state.quit = true
		return "", tea.Quit
	default:
		return fmt.Sprintf("error: unknown command %q", cmd.name), nil
	}
}

func dispatchAppCommand(state *shellState, cmd parsedCommand) (string, tea.Cmd) {
	switch cmd.name {
	case "help", "?":
		if len(cmd.args) > 0 {
			if isHelpAll(cmd.args[0]) {
				return renderAppHelp(), nil
			}
			return renderCommandHelp(cmd.args[0], scopeAppConfig), nil
		}
		return renderAppHelp(), nil
	case "exit", "back":
		setAppContext(state, "")
		return "", nil
	case "run":
		return "", prepareInteractiveCmd(state, shellAction{kind: actionRun, app: state.app})
	case "shell":
		return "", prepareInteractiveCmd(state, shellAction{kind: actionShell, app: state.app})
	case "show":
		return "", runAsync(func() (string, error) {
			return renderAppSummary(state)
		})
	case "snapshot":
		return "", runAsync(func() (string, error) {
			return runAppServerCommand(state, []string{"snapshot"})
		})
	case "snapshots":
		return "", runAsync(func() (string, error) {
			return runAppServerCommand(state, []string{"snapshots"})
		})
	case "restore":
		if len(cmd.args) < 1 {
			return "error: restore requires a snapshot (vN or latest)", nil
		}
		args := []string{"restore", cmd.args[0]}
		return "", runAsync(func() (string, error) {
			return runAppServerCommand(state, args)
		})
	case "update":
		return "", runAsync(func() (string, error) {
			return runAppServerCommand(state, []string{"update"})
		})
	case "delete", "rm":
		return "", shellActionCmd(shellAction{kind: actionDelete, app: state.app})
	case "url":
		return handleURLShell(state, cmd.args)
	case "users":
		return "", shellActionCmd(shellAction{kind: actionUsersEditor, app: state.app})
	default:
		return fmt.Sprintf("error: unknown command %q", cmd.name), nil
	}
}

func handleConfigShell(state *shellState, args []string) (string, tea.Cmd) {
	if len(args) == 0 || args[0] == "show" {
		return renderConfig(state.cfg, state.cfgPath), nil
	}
	if args[0] != "set" || len(args) < 3 {
		return "error: usage: config set host <host> | config set agent <provider>", nil
	}
	switch args[1] {
	case "host":
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		if value == "" {
			return "error: host is required", nil
		}
		state.cfg.DefaultHost = value
		state.host = value
		state.hostPrompt = false
		state.setupNeeded = false
		if err := config.Save(state.cfgPath, state.cfg); err != nil {
			return fmt.Sprintf("error: failed to save config: %v", err), nil
		}
		state.connState = connConnecting
		state.appsLoaded = false
		state.apps = nil
		state.pendingCmd = nil
		closeShellGateway(state)
		return fmt.Sprintf("default host set to %s", value), startHostSync(state)
	case "agent":
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		if value == "" {
			return "error: agent is required", nil
		}
		state.cfg.AgentProvider = value
		state.agent = value
		if err := config.Save(state.cfgPath, state.cfg); err != nil {
			return fmt.Sprintf("error: failed to save config: %v", err), nil
		}
		return fmt.Sprintf("default agent set to %s", value), nil
	default:
		return "error: usage: config set host <host> | config set agent <provider>", nil
	}
}

func handleUsersShell(state *shellState, args []string) (string, tea.Cmd) {
	if len(args) == 0 {
		return "error: usage: users list|add|remove|set-password [host]", nil
	}
	switch args[0] {
	case "list":
		return "", runAsync(func() (string, error) {
			hostArg := ""
			if len(args) > 1 {
				hostArg = args[1]
			}
			return listUsersOutput(state, hostArg)
		})
	case "add", "remove", "set-password":
		username, hostArg, err := parseUsersArgs(args[1:])
		if err != nil {
			return fmt.Sprintf("error: %v", err), nil
		}
		if username == "" {
			return "error: username is required", nil
		}
		switch args[0] {
		case "remove":
			return "", runAsync(func() (string, error) {
				return runUsersRemoveShell(state, username, hostArg)
			})
		case "add":
			return "", shellActionCmd(shellAction{kind: actionUsersAdd, username: username, host: hostArg})
		default:
			return "", shellActionCmd(shellAction{kind: actionUsersSetPassword, username: username, host: hostArg})
		}
	default:
		return "error: usage: users list|add|remove|set-password [host]", nil
	}
}

func handleURLShell(state *shellState, args []string) (string, tea.Cmd) {
	if state.app == "" {
		return "error: no app selected", nil
	}
	if len(args) == 0 || args[0] == "show" {
		return "", runAsync(func() (string, error) {
			return renderURLSummary(state)
		})
	}
	cmd := args[0]
	switch cmd {
	case "open":
		return "", runAsync(func() (string, error) {
			return runURLOpen(state)
		})
	case "public":
		return "", runAsync(func() (string, error) {
			return runURLAccessChange(state, "public")
		})
	case "private":
		return "", runAsync(func() (string, error) {
			return runURLAccessChange(state, "private")
		})
	case "disable":
		return "", runAsync(func() (string, error) {
			return runURLDisabledChange(state, true)
		})
	case "enable":
		return "", runAsync(func() (string, error) {
			return runURLDisabledChange(state, false)
		})
	case "set-domain":
		if len(args) < 2 {
			return "error: url set-domain requires a domain", nil
		}
		return "", runAsync(func() (string, error) {
			return runURLDomainChange(state, args[1], false)
		})
	case "reset-domain":
		return "", runAsync(func() (string, error) {
			return runURLDomainChange(state, "", true)
		})
	default:
		return "error: usage: url [show|open|public|private|disable|enable|set-domain <domain>|reset-domain]", nil
	}
}

func handleSetupShell(state *shellState, args []string) (string, tea.Cmd) {
	if len(args) != 0 {
		return "error: usage: setup", nil
	}
	if state.setupAction != nil {
		return "Setup already in progress.", nil
	}
	state.pendingCmd = nil
	state.setupNeeded = true
	state.setupAction = &setupAction{}
	return renderSetupIntro(state), nil
}

type infoLine struct {
	label string
	desc  string
}

func renderSetupIntro(state *shellState) string {
	theme := shellTheme()
	headerStyle := theme.HelpHeader
	labelStyle := theme.Value
	descStyle := theme.Muted
	if !theme.Enabled {
		headerStyle = lipgloss.NewStyle()
		labelStyle = lipgloss.NewStyle()
		descStyle = lipgloss.NewStyle()
	}
	example := "root@1.2.3.4"
	if theme.Enabled {
		example = theme.Value.Render(example)
	}
	lines := []infoLine{
		{label: "Step 1", desc: "Choose a server (DigitalOcean, Hetzner, or a home server)."},
		{label: "Step 2", desc: "Make sure you can log in (username + IP or hostname)."},
		{label: "Step 3", desc: fmt.Sprintf("Example login: %s", example)},
	}
	header := "Setup: connect your server"
	if theme.Enabled {
		setupStyle := lipgloss.NewStyle().Bold(true)
		restStyle := lipgloss.NewStyle()
		header = fmt.Sprintf("%s %s", setupStyle.Render("Setup:"), restStyle.Render("connect your server"))
		headerStyle = lipgloss.NewStyle()
	}
	return renderInfoTable(header, headerStyle, labelStyle, descStyle, lines)
}

func listUsersOutput(state *shellState, hostArg string) (string, error) {
	gateway, cleanup, err := gatewayForCommand(state, hostArg)
	if err != nil {
		return "", err
	}
	defer cleanup()
	output, err := gateway.command([]string{"proxy", "users", "list"}, "", nil)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(output, "\n"), nil
}

func renderConfig(cfg config.Config, path string) string {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Sprintf("error: failed to render config: %v", err)
	}
	return fmt.Sprintf("Config path: %s\n%s", path, string(data))
}

func renderGlobalHelp(showAll bool) string {
	theme := shellTheme()
	styleCmd := lipgloss.NewStyle()
	styleComment := theme.Muted
	styleHeader := theme.HelpHeader
	hintStyle := theme.Value
	if !theme.Enabled {
		styleComment = lipgloss.NewStyle()
		styleHeader = lipgloss.NewStyle()
		hintStyle = lipgloss.NewStyle()
	}
	if !showAll {
		return renderHelpTable("Commands (use help --all for advanced):", styleHeader, commandLinesForScope(scopeGlobal, false), styleCmd, styleComment, theme.Muted, hintStyle)
	}
	base := commandLinesForScope(scopeGlobal, false)
	advanced := advancedCommandLinesForScope(scopeGlobal)
	rows := []string{""}
	rows = append(rows, renderHelpSection("Commands:", styleHeader, base, styleCmd, styleComment, theme.Muted)...)
	if len(advanced) > 0 {
		rows = append(rows, "")
		rows = append(rows, renderHelpSection("Advanced:", styleHeader, advanced, styleCmd, styleComment, theme.Muted)...)
	}
	rows = append(rows, "")
	rows = append(rows, "Run "+hintStyle.Render("`help <command>`")+" for more details.")
	return strings.Join(rows, "\n")
}

func renderAppHelp() string {
	theme := shellTheme()
	styleCmd := lipgloss.NewStyle()
	styleComment := theme.Muted
	styleHeader := theme.HelpHeader
	if !theme.Enabled {
		styleComment = lipgloss.NewStyle()
		styleHeader = lipgloss.NewStyle()
	}
	return renderHelpTable("Commands:", styleHeader, commandLinesForScope(scopeAppConfig, false), styleCmd, styleComment, theme.Muted, theme.Value)
}

func commandLinesForScope(scope shellScope, includeAdvanced bool) []helpLine {
	specs := commandSpecsForScope(scope)
	lines := make([]helpLine, 0, len(specs))
	for _, spec := range specs {
		if spec.Advanced && !includeAdvanced {
			continue
		}
		lines = append(lines, helpLine{cmd: spec.Display, desc: spec.Summary})
		for _, child := range spec.Children {
			lines = append(lines, helpLine{cmd: child.Cmd, desc: child.Desc, indent: true})
		}
	}
	return lines
}

func advancedCommandLinesForScope(scope shellScope) []helpLine {
	specs := commandSpecsForScope(scope)
	lines := make([]helpLine, 0, len(specs))
	for _, spec := range specs {
		if !spec.Advanced {
			continue
		}
		lines = append(lines, helpLine{cmd: spec.Display, desc: spec.Summary})
		for _, child := range spec.Children {
			lines = append(lines, helpLine{cmd: child.Cmd, desc: child.Desc, indent: true})
		}
	}
	return lines
}

func renderHelpTable(header string, headerStyle lipgloss.Style, lines []helpLine, cmdStyle lipgloss.Style, commentStyle lipgloss.Style, subStyle lipgloss.Style, hintStyle lipgloss.Style) string {
	rows := []string{""}
	rows = append(rows, renderHelpSection(header, headerStyle, lines, cmdStyle, commentStyle, subStyle)...)
	rows = append(rows, "")
	rows = append(rows, "Run "+hintStyle.Render("`help <command>`")+" for more details.")
	return strings.Join(rows, "\n")
}

func renderHelpSection(header string, headerStyle lipgloss.Style, lines []helpLine, cmdStyle lipgloss.Style, commentStyle lipgloss.Style, subStyle lipgloss.Style) []string {
	indentPrefix := "  "
	maxWidth := 0
	for _, line := range lines {
		width := len(line.cmd)
		if line.indent {
			width += len(indentPrefix)
		}
		if width > maxWidth {
			maxWidth = width
		}
	}
	rows := make([]string, 0, len(lines)+1)
	rows = append(rows, headerStyle.Render(header))
	for _, line := range lines {
		width := len(line.cmd)
		if line.indent {
			width += len(indentPrefix)
		}
		padding := maxWidth - width
		if padding < 0 {
			padding = 0
		}
		prefix := "  "
		cmd := line.cmd + strings.Repeat(" ", padding)
		if line.indent {
			prefix += indentPrefix
			rows = append(rows, fmt.Sprintf("%s%s  %s", prefix, subStyle.Render(cmd), subStyle.Render("# "+line.desc)))
			continue
		}
		rows = append(rows, fmt.Sprintf("%s%s  %s", prefix, cmdStyle.Render(cmd), commentStyle.Render("# "+line.desc)))
	}
	return rows
}

func renderInfoTable(header string, headerStyle lipgloss.Style, labelStyle lipgloss.Style, descStyle lipgloss.Style, lines []infoLine) string {
	maxWidth := 0
	for _, line := range lines {
		width := len(line.label)
		if width > maxWidth {
			maxWidth = width
		}
	}
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, "")
	rows = append(rows, headerStyle.Render(header))
	for _, line := range lines {
		padding := maxWidth - len(line.label)
		if padding < 0 {
			padding = 0
		}
		label := line.label + strings.Repeat(" ", padding)
		desc := "# " + line.desc
		rows = append(rows, fmt.Sprintf("  %s  %s", labelStyle.Render(label), descStyle.Render(desc)))
	}
	rows = append(rows, "")
	return strings.Join(rows, "\n")
}

func isHelpAll(arg string) bool {
	switch strings.TrimSpace(strings.ToLower(arg)) {
	case "--all", "-a", "all":
		return true
	default:
		return false
	}
}

func renderCommandHelp(name string, scope shellScope) string {
	spec, ok := lookupCommandSpec(scope, name)
	if !ok && scope == scopeAppConfig {
		spec, ok = lookupCommandSpec(scopeGlobal, name)
	}
	if !ok {
		return fmt.Sprintf("Unknown command: %s", name)
	}
	theme := shellTheme()
	labelStyle := theme.HelpLabel
	usageStyle := theme.HelpUsage
	examplesStyle := theme.HelpExamples
	if !theme.Enabled {
		labelStyle = lipgloss.NewStyle()
		usageStyle = lipgloss.NewStyle()
		examplesStyle = lipgloss.NewStyle()
	}
	lines := []string{""}
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Command:"), spec.Display))
	if strings.TrimSpace(spec.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, spec.Description)
	}
	if strings.TrimSpace(spec.Usage) != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s %s", usageStyle.Render("Usage:"), spec.Usage))
	}
	if len(spec.Options) > 0 {
		lines = append(lines, "")
		lines = append(lines, usageStyle.Render("Options:"))
		for _, opt := range spec.Options {
			lines = append(lines, "  "+opt)
		}
	}
	if len(spec.Examples) > 0 {
		lines = append(lines, "")
		lines = append(lines, examplesStyle.Render("Examples:"))
		for _, ex := range spec.Examples {
			lines = append(lines, "  "+ex)
		}
	}
	return strings.Join(lines, "\n")
}

func renderAppSummary(state *shellState) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	status := "unknown"
	agentProviderForChecks := strings.TrimSpace(state.agent)
	if agentProviderForChecks == "" {
		agentProviderForChecks = agents.DefaultProvider()
	}
	exists, err := remoteContainerExists(state.gateway, resolved.App, agentProviderForChecks)
	if err == nil {
		if exists {
			status = "running"
		} else {
			status = "not running"
		}
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		info = proxyInfo{App: resolved.App}
	}
	buf := &ttyBuffer{tty: true}
	fmt.Fprintf(buf, "App: %s\n", resolved.App)
	fmt.Fprintf(buf, "Status: %s\n", status)
	if info.URL != "" {
		fmt.Fprintf(buf, "URL: %s\n", info.URL)
	}
	if info.Access != "" {
		fmt.Fprintf(buf, "Access: %s\n", info.Access)
	}
	return buf.String(), nil
}

func renderURLSummary(state *shellState) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		return "", err
	}
	buf := &ttyBuffer{tty: true}
	printShellURLSummary(buf, info)
	return buf.String(), nil
}

func runURLAccessChange(state *shellState, access string) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	if err := runRemoteSetAccess(state.gateway, resolved.App, access); err != nil {
		return "", err
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		return "", err
	}
	buf := &ttyBuffer{tty: true}
	printShellURLActionResult(buf, info)
	return buf.String(), nil
}

func runURLDisabledChange(state *shellState, disabled bool) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	if err := runRemoteSetDisabled(state.gateway, resolved.App, disabled); err != nil {
		return "", err
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		return "", err
	}
	buf := &ttyBuffer{tty: true}
	printShellURLActionResult(buf, info)
	return buf.String(), nil
}

func runURLDomainChange(state *shellState, domain string, reset bool) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	if err := runRemoteSetDomain(state.gateway, resolved.App, domain, reset); err != nil {
		return "", err
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		return "", err
	}
	buf := &ttyBuffer{tty: true}
	printShellURLActionResult(buf, info)
	return buf.String(), nil
}

func runAppServerCommand(state *shellState, args []string) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	remoteArgs := buildAppCommandArgs(strings.TrimSpace(state.agent), resolved.App, args)
	output, err := state.gateway.command(remoteArgs, "", nil)
	if err != nil {
		return "", err
	}
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return "OK", nil
	}
	return output, nil
}

func resolveAppTarget(state *shellState) (target.Resolved, error) {
	app := strings.TrimSpace(state.app)
	if app == "" {
		return target.Resolved{}, errors.New("no app selected")
	}
	cfg := state.cfg
	if state.host != "" {
		cfg.DefaultHost = state.host
	}
	return target.Resolve(app, cfg)
}

func runAsync(fn func() (string, error)) tea.Cmd {
	return func() tea.Msg {
		output, err := fn()
		return commandResultMsg{output: output, err: err}
	}
}

func shellActionCmd(action shellAction) tea.Cmd {
	return func() tea.Msg {
		return shellActionMsg{action: action}
	}
}

func prepareInteractiveCmd(state *shellState, action shellAction) tea.Cmd {
	return func() tea.Msg {
		actionArg := ""
		if action.kind == actionShell {
			actionArg = "shell"
		}
		session, fallback, err := prepareInteractiveSession(state, action.app, actionArg)
		if err != nil {
			return interactivePreparedMsg{action: action, err: err}
		}
		if fallback || session == nil {
			return interactivePreparedMsg{action: action, fallback: true}
		}
		return interactivePreparedMsg{action: action, session: session}
	}
}

func waitCmd() tea.Cmd {
	return func() tea.Msg {
		return nil
	}
}

func loadAppsCmd(state *shellState, render bool) tea.Cmd {
	return func() tea.Msg {
		if state == nil {
			return appsLoadedMsg{err: errors.New("missing shell state"), render: render}
		}
		if state.syncing || !gatewayReady(state) {
			return appsLoadedMsg{err: errors.New("gateway not connected"), render: render, fromAppsCmd: render}
		}
		gateway, cleanup, err := gatewayForCommand(state, "")
		if err != nil {
			return appsLoadedMsg{err: err, render: render, fromAppsCmd: render}
		}
		defer cleanup()
		apps, err := runRemoteAppsList(gateway)
		return appsLoadedMsg{apps: apps, err: err, render: render, fromAppsCmd: render}
	}
}

func parseUsersArgs(args []string) (string, string, error) {
	username := ""
	host := ""
	for i := 0; i < len(args); i++ {
		part := strings.TrimSpace(args[i])
		if part == "" {
			continue
		}
		if part == "--username" {
			if i+1 >= len(args) {
				return "", "", errors.New("missing value for --username")
			}
			username = strings.TrimSpace(args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(part, "--") {
			return "", "", fmt.Errorf("unknown flag: %s", part)
		}
		if host == "" {
			host = part
			continue
		}
		return "", "", fmt.Errorf("unexpected argument: %s", part)
	}
	return username, host, nil
}

func runUsersRemoveShell(state *shellState, username string, hostArg string) (string, error) {
	gateway, cleanup, err := gatewayForCommand(state, hostArg)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if err := runRemoteUsersRemove(gateway, username); err != nil {
		return "", err
	}
	return "OK", nil
}

func runURLOpen(state *shellState) (string, error) {
	resolved, err := resolveAppTarget(state)
	if err != nil {
		return "", err
	}
	if state.gateway == nil {
		return "", errors.New("gateway not connected")
	}
	info, err := fetchProxyInfo(state.gateway, resolved.App)
	if err != nil {
		return "", err
	}
	if info.Disabled || strings.TrimSpace(info.URL) == "" {
		return "", fmt.Errorf("URL is not available")
	}
	if err := openURL(info.URL); err != nil {
		return "", err
	}
	return "", nil
}

func parseShellCommand(line string) (parsedCommand, error) {
	fields, err := splitShellArgs(line)
	if err != nil {
		return parsedCommand{}, err
	}
	if len(fields) == 0 {
		return parsedCommand{}, nil
	}
	return parsedCommand{name: strings.ToLower(fields[0]), args: fields[1:]}, nil
}

func splitShellArgs(input string) ([]string, error) {
	var out []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		out = append(out, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == ' ' || r == '\t':
			if inSingle || inDouble {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped || inSingle || inDouble {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return out, nil
}

func syncHostCmd(state *shellState) tea.Cmd {
	host := state.host
	cfg := state.cfg
	agent := strings.TrimSpace(state.agent)
	return func() tea.Msg {
		if strings.TrimSpace(host) == "" {
			return hostSyncMsg{reachable: false, bootstrapped: false, err: errors.New("no host configured")}
		}
		resolved, err := target.ResolveHost(host, cfg)
		if err != nil {
			return hostSyncMsg{reachable: false, bootstrapped: false, err: err}
		}
		gateway, err := startGateway(resolved.Host, agent, nil, false)
		if err != nil {
			if isMissingServerError(err) {
				return hostSyncMsg{reachable: true, bootstrapped: false, err: nil}
			}
			return hostSyncMsg{reachable: false, bootstrapped: false, err: err}
		}
		apps, err := runRemoteAppsList(gateway)
		if err != nil {
			_ = gateway.Close()
			return hostSyncMsg{reachable: true, bootstrapped: true, err: err}
		}
		return hostSyncMsg{reachable: true, bootstrapped: true, apps: apps, gateway: gateway, gatewayHost: resolved.Host, err: nil}
	}
}

func printShellURLSummary(out *ttyBuffer, info proxyInfo) {
	styler := newURLStyler(out)
	fmt.Fprintf(out, "%s %s\n", styler.label("App:"), styler.value(info.App))
	if info.Disabled {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("disabled (no URL)"))
	} else if info.Access == "public" {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("public"))
	} else {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("requires login"))
	}
	if info.URL != "" {
		fmt.Fprintf(out, "%s %s\n", styler.label("URL:"), styler.link(info.URL))
	}
	if info.URL != "" && info.PublicIP != "" {
		host := strings.TrimPrefix(info.URL, "https://")
		host = strings.TrimPrefix(host, "http://")
		if host != "" {
			fmt.Fprintf(out, "%s %s\n", styler.label("DNS:"), styler.dnsLine(host, info.PublicIP))
		}
	}
	if info.CustomDomain != "" {
		fmt.Fprintf(out, "%s %s\n", styler.label("Custom domain:"), styler.value(info.CustomDomain))
	}
	if len(info.AllowedUsers) > 0 {
		fmt.Fprintf(out, "%s %s\n", styler.label("Users:"), styler.value(strings.Join(info.AllowedUsers, ", ")))
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, styler.header("Commands:"))
	styler.commands(out, []commandLine{
		{cmd: "url set-domain <domain>", desc: "set a full domain (e.g., myblog.com)"},
		{cmd: "url reset-domain", desc: "reset to the default domain"},
		{cmd: "users", desc: "manage who can access this app"},
		{cmd: "url public", desc: "allow anyone to access"},
		{cmd: "url private", desc: "require login to access"},
		{cmd: "url disable", desc: "turn off the URL"},
		{cmd: "url enable", desc: "turn the URL back on"},
	})
}

func printShellURLActionResult(out *ttyBuffer, info proxyInfo) {
	styler := newURLStyler(out)
	status := "requires login"
	if info.Disabled {
		status = "disabled"
	} else if info.Access == "public" {
		status = "public"
	}
	fmt.Fprintf(out, "%s %s\n", styler.label("URL access:"), styler.status(status))
	if info.URL != "" {
		fmt.Fprintf(out, "%s %s\n", styler.label("URL:"), styler.link(info.URL))
	}
}

// ttyBuffer allows lipgloss rendering while capturing output.
type ttyBuffer struct {
	bytes.Buffer
	tty bool
}

func (b *ttyBuffer) IsTTY() bool {
	return b.tty
}
