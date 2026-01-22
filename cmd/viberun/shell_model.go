// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type shellModel struct {
	state      *shellState
	input      textinput.Model
	promptSpin spinner.Model
	width      int
	height     int
	busy       bool
}

type commandResultMsg struct {
	output string
	err    error
}

type hostSyncMsg struct {
	reachable    bool
	bootstrapped bool
	apps         []string
	gateway      *gatewayClient
	gatewayHost  string
	err          error
}

type shellActionMsg struct {
	action shellAction
}

type interactivePreparedMsg struct {
	action   shellAction
	session  *preparedSession
	err      error
	fallback bool
}

type appsLoadedMsg struct {
	apps        []string
	render      bool
	err         error
	fromAppsCmd bool
}

func newShellModel(state *shellState) shellModel {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = ""
	input.Focus()
	input.CharLimit = 0
	input.Width = 80

	spin := spinner.New()
	spin.Spinner = spinner.Spinner{Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, FPS: 120 * time.Millisecond}

	model := shellModel{
		state:      state,
		input:      input,
		promptSpin: spin,
	}
	if len(state.output) == 0 {
		model.state.appendOutput(renderBanner(state))
		if state.setupNeeded {
			model.state.setupHinted = true
		}
	}
	if state.connState == connUnknown && !state.hostPrompt {
		model.state.connState = connConnecting
	}
	return model
}

func (m shellModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.promptSpin.Tick}
	if !m.state.hostPrompt {
		m.state.appsLoaded = false
		m.state.apps = nil
		m.state.syncing = true
		cmds = append(cmds, syncHostCmd(m.state))
	}
	return tea.Batch(cmds...)
}

func (m shellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			// Match shell behavior: clear the current input without exiting.
			m.busy = false
			m.input.SetValue("")
			m.input.CursorEnd()
			return m, nil
		}
		if msg.Type == tea.KeyCtrlL {
			// Clear screen: keep header + prompt, drop scrollback.
			m.state.output = nil
			return m, tea.ClearScreen
		}
		if msg.Type == tea.KeyCtrlD {
			if strings.TrimSpace(m.input.Value()) == "" {
				m.state.quit = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.busy {
			return m, nil
		}
		switch msg.Type {
		case tea.KeyUp:
			m.historyPrev()
			return m, nil
		case tea.KeyDown:
			m.historyNext()
			return m, nil
		case tea.KeyEnter:
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if line == "" {
				// Empty Enter should emit a prompt line like a real shell.
				m.appendCommandLine("")
				return m, nil
			}
			m.appendCommandLine(line)
			m.state.history = append(m.state.history, line)
			m.state.historyIdx = len(m.state.history)
			result, cmd := dispatchShellCommand(m.state, line)
			if cmd != nil {
				m.busy = true
				return m, cmd
			}
			if m.state.pendingCmd != nil {
				m.busy = true
				return m, nil
			}
			if result != "" {
				m.state.appendOutput(result)
			}
			if m.state.setupAction != nil {
				return m, tea.Quit
			}
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.promptSpin, cmd = m.promptSpin.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 0 {
			m.input.Width = m.width - 6
			if m.input.Width < 10 {
				m.input.Width = 10
			}
		}
		return m, nil
	case commandResultMsg:
		m.busy = false
		if msg.output != "" {
			m.state.appendOutput(msg.output)
		}
		if msg.err != nil {
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
		}
		return m, nil
	case hostSyncMsg:
		m.state.syncing = false
		m.busy = false
		if msg.gateway != nil {
			closeShellGateway(m.state)
			m.state.gateway = msg.gateway
			m.state.gatewayHost = msg.gatewayHost
		} else if msg.err != nil || !msg.bootstrapped {
			closeShellGateway(m.state)
		}
		if msg.err != nil {
			m.state.connState = connFailed
			m.state.connError = msg.err.Error()
			m.state.bootstrapped = false
			m.state.appsLoaded = false
			m.state.apps = nil
			m.state.appendOutput(fmt.Sprintf("Host check failed: %v", msg.err))
			m.state.pendingCmd = nil
			return m, nil
		}
		if !msg.reachable {
			m.state.connState = connFailed
			m.state.connError = "unreachable"
			m.state.bootstrapped = false
			m.state.appsLoaded = false
			m.state.apps = nil
			m.state.appendOutput("Host unreachable.")
			m.state.pendingCmd = nil
			return m, nil
		}
		m.state.connState = connConnected
		m.state.connError = ""
		m.state.bootstrapped = msg.bootstrapped
		if !msg.bootstrapped {
			m.state.setupNeeded = true
			m.state.appsLoaded = false
			m.state.appsSyncing = false
			m.state.apps = nil
			m.state.pendingCmd = nil
			if !m.state.setupHinted {
				m.state.appendOutput(renderSetupBanner())
				m.state.setupHinted = true
			}
			return m, nil
		}
		m.state.setupNeeded = false
		m.state.apps = msg.apps
		m.state.appsLoaded = true
		m.state.appsSyncing = false
		if m.state.pendingCmd != nil {
			pending := m.state.pendingCmd
			m.state.pendingCmd = nil
			result, cmd := dispatchCommandWithScope(m.state, pending)
			if cmd != nil {
				m.busy = true
				return m, cmd
			}
			if result != "" {
				m.state.appendOutput(result)
			}
		}
		return m, nil
	case shellActionMsg:
		m.state.shellAction = &msg.action
		m.busy = false
		return m, tea.Quit
	case interactivePreparedMsg:
		m.busy = false
		if msg.err != nil {
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
			return m, nil
		}
		if msg.fallback {
			m.state.shellAction = &msg.action
			return m, tea.Quit
		}
		if msg.session == nil {
			m.state.appendOutput("error: failed to prepare session")
			return m, nil
		}
		m.state.preparedSession = msg.session
		return m, tea.Quit
	case appsLoadedMsg:
		m.state.appsSyncing = false
		m.busy = false
		if msg.err != nil {
			if msg.fromAppsCmd && (m.state.syncing || !gatewayReady(m.state)) {
				m.state.pendingCmd = &pendingCommand{cmd: parsedCommand{name: "apps"}, scope: scopeGlobal}
				if !m.state.syncing {
					return m, startHostSync(m.state)
				}
				return m, nil
			}
			if msg.fromAppsCmd || m.state.appsRenderPending {
				m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
			}
			m.state.appsRenderPending = false
			return m, nil
		}
		m.state.apps = msg.apps
		m.state.appsLoaded = true
		if msg.render || m.state.appsRenderPending {
			m.state.appendOutput(renderAppsList(m.state))
		}
		m.state.appsRenderPending = false
		if m.state.pendingCmd != nil {
			pending := m.state.pendingCmd
			m.state.pendingCmd = nil
			result, cmd := dispatchCommandWithScope(m.state, pending)
			if cmd != nil {
				m.busy = true
				return m, cmd
			}
			if result != "" {
				m.state.appendOutput(result)
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m shellModel) View() string {
	header := renderHeader(m.state)
	outputLines := m.state.output
	availableHeight := m.height - countLines(header) - 1
	if availableHeight < 0 {
		availableHeight = 0
	}
	if len(outputLines) > availableHeight {
		outputLines = outputLines[len(outputLines)-availableHeight:]
	}
	body := strings.Join(outputLines, "\n")
	if body == "" {
		if m.busy {
			return header + "\n" + renderLoadingLine(m)
		}
		lines := []string{header, ""}
		lines = append(lines, renderPrompt(m))
		return strings.Join(lines, "\n")
	}
	prefix := renderPromptPrefix(m.state)
	separator := "\n"
	if len(outputLines) > 0 && strings.HasPrefix(outputLines[0], prefix) {
		// After a clear, keep a blank line between the header and first prompt line.
		separator = "\n\n"
	}
	if m.busy {
		// Preserve the same header spacing even while the spinner is visible.
		return header + separator + body + "\n" + renderLoadingLine(m)
	}
	lines := []string{header, body}
	if separator == "\n\n" {
		lines = []string{header, "", body}
	}
	lastLine := ""
	if len(outputLines) > 0 {
		lastLine = outputLines[len(outputLines)-1]
	}
	if len(outputLines) == 0 || (lastLine != "" && lastLine != prefix) {
		// Keep a blank line before the prompt unless we just printed a prompt line.
		lines = append(lines, "")
	}
	lines = append(lines, renderPrompt(m))
	return strings.Join(lines, "\n")
}

func (m *shellModel) appendCommandLine(line string) {
	prefix := renderPromptPrefix(m.state)
	if len(m.state.output) > 0 {
		lastLine := m.state.output[len(m.state.output)-1]
		if lastLine != "" && lastLine != prefix {
			// Preserve the visual blank line that was shown before the prompt.
			m.state.appendOutput("")
		}
	}
	m.state.appendOutput(prefix + line)
}

func (m *shellModel) historyPrev() {
	if len(m.state.history) == 0 {
		return
	}
	idx := m.state.historyIdx
	if idx <= 0 {
		idx = 0
	} else {
		idx--
	}
	m.state.historyIdx = idx
	m.input.SetValue(m.state.history[idx])
	m.input.CursorEnd()
}

func (m *shellModel) historyNext() {
	if len(m.state.history) == 0 {
		return
	}
	idx := m.state.historyIdx
	if idx >= len(m.state.history)-1 {
		m.state.historyIdx = len(m.state.history)
		m.input.SetValue("")
		return
	}
	idx++
	m.state.historyIdx = idx
	m.input.SetValue(m.state.history[idx])
	m.input.CursorEnd()
}

func renderBanner(state *shellState) string {
	if state != nil && state.setupNeeded {
		return renderSetupBanner()
	}
	return renderRunBanner()
}

func renderRunBanner() string {
	theme := shellTheme()
	if !theme.Enabled {
		return "\n- run appname to start building • help\n"
	}
	return fmt.Sprintf("\n- %s %s to start building • %s\n", theme.BannerVerb.Render("run"), theme.BannerApp.Render("appname"), theme.BannerHelp.Render("help"))
}

func renderSetupBanner() string {
	theme := shellTheme()
	if !theme.Enabled {
		return "\n- Welcome to viberun. Type setup to connect your server.\n"
	}
	setup := theme.BannerVerb.Render("setup")
	return fmt.Sprintf("\n- Welcome to viberun. Type %s to connect your server.\n", setup)
}

func renderFirstAppHint() string {
	theme := shellTheme()
	if !theme.Enabled {
		return "Create your first app: run myapp"
	}
	return fmt.Sprintf("Create your first app: %s %s", theme.BannerVerb.Render("run"), theme.BannerApp.Render("myapp"))
}

func renderHeader(state *shellState) string {
	theme := shellTheme()
	if !theme.Enabled {
		host := state.host
		if host == "" {
			host = "<unset>"
		}
		agent := state.agent
		if agent == "" {
			agent = "default"
		}
		status := "○"
		switch state.connState {
		case connConnected:
			status = "●"
		case connFailed:
			status = "○"
		case connConnecting:
			status = "○"
		}
		return fmt.Sprintf("%s viberun host=%s agent=%s", status, host, agent)
	}
	status := "○"
	statusStyle := theme.StatusConnecting
	switch state.connState {
	case connConnected:
		status = "●"
		statusStyle = theme.StatusConnected
	case connFailed:
		status = "○"
		statusStyle = theme.StatusFailed
	case connConnecting:
		status = "○"
		statusStyle = theme.StatusConnecting
	}
	host := state.host
	if host == "" {
		host = "<unset>"
	}
	agent := state.agent
	if agent == "" {
		agent = "default"
	}
	return fmt.Sprintf("%s %s  %s %s  %s %s",
		statusStyle.Render(status),
		theme.Brand.Render("viberun"),
		theme.Label.Render("host="),
		theme.Value.Render(host),
		theme.Label.Render("agent="),
		theme.Value.Render(agent),
	)
}

func renderPrompt(m shellModel) string {
	return renderPromptPrefix(m.state) + m.input.View() + " "
}

func renderLoadingLine(m shellModel) string {
	return m.promptSpin.View()
}

func renderPromptPrefix(state *shellState) string {
	theme := shellTheme()
	if !theme.Enabled {
		if state.scope == scopeAppConfig && state.app != "" {
			return fmt.Sprintf("viberun %s > ", state.app)
		}
		return "viberun > "
	}
	brand := theme.PromptBrand.Render("viberun")
	arrow := theme.PromptArrow.Render(">")
	if state.scope == scopeAppConfig && state.app != "" {
		app := theme.Value.Render(state.app)
		return fmt.Sprintf("%s %s %s ", brand, app, arrow)
	}
	return fmt.Sprintf("%s %s ", brand, arrow)
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
