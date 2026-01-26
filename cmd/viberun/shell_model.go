// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shayne/viberun/internal/tui/dialogs"
	"github.com/shayne/viberun/internal/tui/theme"
)

type shellModel struct {
	state            *shellState
	input            textinput.Model
	promptSpin       spinner.Model
	width            int
	height           int
	busy             bool
	refreshingTheme  bool
	lastThemeRefresh time.Time
	lastPaletteVer   uint64
	oscState         oscParseState
	oscCodeLen       int
	oscStartedAt     time.Time
}

type commandResultMsg struct {
	output string
	err    error
}

type oscParseState int

const (
	oscStateIdle oscParseState = iota
	oscStateCode
	oscStateData
)

const oscDiscardTimeout = 250 * time.Millisecond
const themePollInterval = 1 * time.Second

type hostSyncMsg struct {
	reachable    bool
	bootstrapped bool
	apps         []appSummary
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
	apps        []appSummary
	render      bool
	err         error
	fromAppsCmd bool
}

type startupConnectMsg struct {
	reachable    bool
	bootstrapped bool
	gateway      *gatewayClient
	gatewayHost  string
	err          error
}

type startupAppsMsg struct {
	apps         []appSnapshot
	proxyEnabled bool
	err          error
}

type startupReadyMsg struct {
	apps []appSummary
	err  error
}

type appsStreamStartedMsg struct {
	stream *appsStream
	err    error
}

type appsStreamUpdateMsg struct {
	apps []appSnapshot
	err  error
}

type themePollMsg struct{}

func newShellModel(state *shellState) shellModel {
	state.headerRendered = false

	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = ""
	input.CharLimit = 0
	input.SetWidth(80)
	input.Focus()

	spin := spinner.New()
	spin.Spinner = spinner.Spinner{Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, FPS: 120 * time.Millisecond}
	spin.Style = theme.ForShell().Shell.StatusConnecting
	initialTheme := theme.ForShell()

	model := shellModel{
		state:           state,
		input:           input,
		promptSpin:      spin,
		refreshingTheme: true,
		lastPaletteVer:  initialTheme.Palette.Version,
	}
	if len(state.output) == 0 {
		if state.hostPrompt {
			model.state.appendOutput(renderSetupBanner())
			model.state.setupHinted = true
		}
	}
	if state.connState == connUnknown && !state.hostPrompt {
		model.state.connState = connConnecting
		model.state.startupActive = true
		model.state.startupStage = fmt.Sprintf("Connecting to %s...", hostLabel(state.host))
	}
	return model
}

func (m shellModel) Init() tea.Cmd {
	cmds := []tea.Cmd{}
	if m.state.clearOnStart {
		m.state.clearOnStart = false
		cmds = append(cmds, tea.ClearScreen)
	}
	if cmd := m.input.Focus(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Kick off a theme refresh early so startup text can use the latest palette ASAP.
	cmds = append(cmds, refreshShellThemeCmd())
	cmds = append(cmds, themePollCmd())
	if m.state.startupActive {
		cmds = append(cmds, spinnerTickCmd(m.promptSpin))
	}
	if m.state.resumeAfterAction && gatewayReady(m.state) {
		m.state.resumeAfterAction = false
		startForwardManager(m.state)
		m.state.appsSyncing = true
		if cmd := startAppsStreamCmd(m.state); cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, loadAppsCmd(m.state, false))
		return tea.Batch(cmds...)
	}
	if !m.state.hostPrompt {
		m.state.appsLoaded = false
		m.state.apps = nil
		m.state.syncing = true
		cmds = append(cmds, startupConnectCmd(m.state))
	}
	return tea.Batch(cmds...)
}

func (m shellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state != nil && m.state.promptFlow != nil {
		dialog := m.state.promptDialog
		if dialog == nil {
			dialog = m.state.promptFlow.Dialog()
		}
		updated, cmd := dialog.Update(msg)
		m.state.promptDialog = updated
		if result, ok := dialogResult(updated); ok {
			m.state.promptFlow.ApplyResult(*result)
			if m.state.promptFlow.Done() || m.state.promptFlow.Cancelled() {
				note, quit := applyPromptFlowResult(m.state, m.state.promptFlow)
				m.state.promptFlow = nil
				m.state.promptDialog = nil
				if note != "" {
					m.state.appendOutput(note)
				}
				if quit {
					return m, tea.Quit
				}
			} else {
				m.state.promptDialog = m.state.promptFlow.Dialog()
				if m.state.promptDialog != nil {
					if initCmd := m.state.promptDialog.Init(); initCmd != nil {
						cmd = tea.Batch(cmd, initCmd)
					}
				}
			}
		}
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.handleOSCKey(msg) {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			// Match shell behavior: clear the current input without exiting.
			m.busy = false
			if m.state != nil {
				m.state.busyLabel = ""
			}
			m.input.SetValue("")
			m.input.CursorEnd()
			return m, nil
		case "ctrl+l":
			// Clear screen: keep header + prompt, drop scrollback.
			m.state.output = nil
			m.state.headerRendered = false
			return m, tea.ClearScreen
		case "ctrl+d":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.state.quit = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.busy {
			return m, nil
		}
		if m.state.startupActive {
			return m, nil
		}
		switch msg.String() {
		case "up":
			m.historyPrev()
			return m, nil
		case "down":
			m.historyNext()
			return m, nil
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if m.state.confirmDeleteApp != "" {
				app := m.state.confirmDeleteApp
				m.state.confirmDeleteApp = ""
				response := strings.ToLower(strings.TrimSpace(line))
				promptLine := confirmDeletePrompt(m.state)
				if promptLine == "" {
					promptLine = fmt.Sprintf("Delete %s and all snapshots? [y/N]: ", app)
				}
				m.state.appendOutput(promptLine + strings.TrimSpace(line))
				if response == "y" || response == "yes" {
					m.busy = true
					return m, m.startSpinner(runAsync(func() (string, error) {
						return runDeleteConfirmed(m.state, app)
					}))
				}
				m.state.appendOutput("delete cancelled")
				return m, m.maybeRefreshTheme()
			}
			if line == "" {
				// Empty Enter should emit a prompt line like a real shell.
				m.appendCommandLine("")
				return m, m.maybeRefreshTheme()
			}
			m.appendCommandLine(line)
			m.state.history = append(m.state.history, line)
			m.state.historyIdx = len(m.state.history)
			result, cmd := dispatchShellCommand(m.state, line)
			if m.state.promptFlow != nil {
				if result != "" {
					m.state.appendOutput(result)
				}
				return m, cmd
			}
			if cmd != nil {
				m.busy = true
				return m, m.startSpinner(cmd)
			}
			if m.state.pendingCmd != nil {
				m.busy = true
				return m, m.startSpinner(nil)
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
		if m.busy || m.state.startupActive {
			return m, cmd
		}
		return m, cmd
	case themePollMsg:
		cmds := []tea.Cmd{themePollCmd()}
		if !m.busy && !m.state.startupActive {
			if refresh := m.maybeRefreshTheme(); refresh != nil {
				cmds = append(cmds, refresh)
			}
		}
		return m, tea.Batch(cmds...)
	case themeRefreshMsg:
		m.refreshingTheme = false
		m.lastThemeRefresh = time.Now()
		updatedTheme := theme.ForShell()
		m.promptSpin.Style = updatedTheme.Shell.StatusConnecting
		if updatedTheme.Palette.Version != m.lastPaletteVer {
			m.lastPaletteVer = updatedTheme.Palette.Version
			if cmd := repaintCmd(m.width, m.height); cmd != nil {
				return m, cmd
			}
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 0 {
			width := m.width - 6
			if width < 10 {
				width = 10
			}
			m.input.SetWidth(width)
		}
		return m, nil
	case commandResultMsg:
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
		if msg.output != "" {
			m.state.appendOutput(msg.output)
		}
		if msg.err != nil {
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
		}
		return m, m.maybeRefreshTheme()
	case startupConnectMsg:
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
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
			m.state.startupActive = false
			m.state.startupStage = ""
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
			m.state.startupActive = false
			m.state.startupStage = ""
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
			m.state.startupActive = false
			m.state.startupStage = ""
			if !m.state.setupHinted {
				m.state.appendOutput(renderSetupBanner())
				m.state.setupHinted = true
			}
			return m, nil
		}
		m.state.setupNeeded = false
		startForwardManager(m.state)
		m.state.startupStage = "Syncing apps..."
		return m, startupAppsCmd(m.state)
	case startupAppsMsg:
		if msg.err != nil {
			m.state.startupActive = false
			m.state.startupStage = ""
			m.state.syncing = false
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
			return m, nil
		}
		m.state.startupStage = "Forwarding app ports..."
		return m, startupForwardCmd(m.state, msg.apps, msg.proxyEnabled)
	case startupReadyMsg:
		m.state.startupActive = false
		m.state.startupStage = ""
		m.state.syncing = false
		if msg.err != nil {
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
			return m, nil
		}
		startForwardManager(m.state)
		if m.state.forwarder != nil {
			m.state.forwarder.syncFromSummaries(msg.apps)
		}
		m.state.apps = applyAppSummaries(m.state, msg.apps)
		m.state.appsLoaded = true
		m.state.appsSyncing = false
		if !m.state.startupRendered {
			m.state.appendStartupOutput(renderStartupSummary(m.state))
			m.state.startupRendered = true
		}
		cmds := []tea.Cmd{}
		if cmd := startAppsStreamCmd(m.state); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.state.pendingCmd != nil {
			pending := m.state.pendingCmd
			m.state.pendingCmd = nil
			result, cmd := dispatchCommandWithScope(m.state, pending)
			if cmd != nil {
				m.busy = true
				cmds = append(cmds, cmd)
			}
			if result != "" {
				m.state.appendOutput(result)
			}
		}
		if m.busy {
			cmds = append(cmds, m.promptSpin.Tick)
		}
		if cmd := m.maybeRefreshTheme(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if len(cmds) == 0 {
			return m, nil
		}
		return m, tea.Batch(cmds...)
	case hostSyncMsg:
		m.state.syncing = false
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
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
		startForwardManager(m.state)
		if m.state.forwarder != nil {
			m.state.forwarder.syncFromSummaries(msg.apps)
		}
		m.state.apps = applyAppSummaries(m.state, msg.apps)
		m.state.appsLoaded = true
		m.state.appsSyncing = false
		if m.state.pendingCmd != nil {
			pending := m.state.pendingCmd
			m.state.pendingCmd = nil
			result, cmd := dispatchCommandWithScope(m.state, pending)
			if cmd != nil {
				m.busy = true
				return m, m.startSpinner(cmd)
			}
			if result != "" {
				m.state.appendOutput(result)
			}
		}
		return m, startAppsStreamCmd(m.state)
	case shellActionMsg:
		m.state.shellAction = &msg.action
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
		return m, tea.Quit
	case interactivePreparedMsg:
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
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
	case appsStreamStartedMsg:
		if msg.err != nil {
			m.state.appendOutput(fmt.Sprintf("error: %v", msg.err))
			return m, nil
		}
		m.state.appsStream = msg.stream
		return m, listenAppsStreamCmd(msg.stream)
	case appsStreamUpdateMsg:
		if msg.err != nil {
			if m.state.appsStream != nil {
				if m.state.appsStream.close != nil {
					m.state.appsStream.close()
				}
				m.state.appsStream = nil
			}
			return m, nil
		}
		m.state.appsSyncing = true
		cmds := []tea.Cmd{refreshAppsFromStreamCmd(m.state, msg.apps)}
		if m.state.appsStream != nil {
			cmds = append(cmds, listenAppsStreamCmd(m.state.appsStream))
		}
		return m, tea.Batch(cmds...)
	case appsLoadedMsg:
		m.state.appsSyncing = false
		m.busy = false
		if m.state != nil {
			m.state.busyLabel = ""
		}
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
		if m.state.forwarder != nil {
			m.state.forwarder.syncFromSummaries(msg.apps)
		}
		m.state.apps = applyAppSummaries(m.state, msg.apps)
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
				return m, m.startSpinner(cmd)
			}
			if result != "" {
				m.state.appendOutput(result)
			}
		}
		return m, m.maybeRefreshTheme()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *shellModel) startSpinner(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return spinnerTickCmd(m.promptSpin)
	}
	return tea.Batch(cmd, spinnerTickCmd(m.promptSpin))
}

func spinnerTickCmd(spin spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return spin.Tick()
	}
}

func (m *shellModel) maybeRefreshTheme() tea.Cmd {
	if m.refreshingTheme || m.state.startupActive || m.busy {
		return nil
	}
	if !m.lastThemeRefresh.IsZero() && time.Since(m.lastThemeRefresh) < 500*time.Millisecond {
		return nil
	}
	m.refreshingTheme = true
	return refreshShellThemeCmd()
}

func themePollCmd() tea.Cmd {
	return tea.Tick(themePollInterval, func(time.Time) tea.Msg {
		return themePollMsg{}
	})
}

func repaintCmd(width, height int) tea.Cmd {
	if width <= 0 || height <= 0 {
		return nil
	}
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: width, Height: height}
	}
}

func (m *shellModel) handleOSCKey(msg tea.KeyPressMsg) bool {
	now := time.Now()
	key := msg.Key()
	if m.oscState == oscStateIdle {
		if isOSCStartKey(key) {
			m.oscState = oscStateCode
			m.oscCodeLen = 0
			m.oscStartedAt = now
			return true
		}
		return false
	}
	if !m.oscStartedAt.IsZero() && now.Sub(m.oscStartedAt) > oscDiscardTimeout {
		m.resetOSC()
		return false
	}
	if isCtrlG(key) || isOSCEndKey(key) {
		m.resetOSC()
		return true
	}
	if key.Mod&tea.ModAlt != 0 || utf8.RuneCountInString(key.Text) != 1 {
		m.resetOSC()
		return false
	}
	r, _ := utf8.DecodeRuneInString(key.Text)
	switch m.oscState {
	case oscStateCode:
		if r >= '0' && r <= '9' {
			m.oscCodeLen++
			return true
		}
		if r == ';' && m.oscCodeLen > 0 {
			m.oscState = oscStateData
			return true
		}
		m.resetOSC()
		return false
	case oscStateData:
		if isOSCDataRune(r) {
			return true
		}
		m.resetOSC()
		return false
	default:
		m.resetOSC()
		return false
	}
}

func isOSCStartKey(key tea.Key) bool {
	if key.Mod&tea.ModAlt == 0 || utf8.RuneCountInString(key.Text) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(key.Text)
	return r == ']'
}

func isOSCEndKey(key tea.Key) bool {
	if key.Mod&tea.ModAlt == 0 || utf8.RuneCountInString(key.Text) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(key.Text)
	return r == '\\'
}

func isCtrlG(key tea.Key) bool {
	if key.Mod&tea.ModCtrl == 0 {
		return false
	}
	return key.Code == 'g' || key.Code == 'G'
}

func isOSCDataRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f':
		return true
	case r >= 'A' && r <= 'F':
		return true
	}
	switch r {
	case 'r', 'g', 'b', 'R', 'G', 'B':
		return true
	case '#', ':', '/', '?':
		return true
	}
	return false
}

func (m *shellModel) resetOSC() {
	m.oscState = oscStateIdle
	m.oscCodeLen = 0
	m.oscStartedAt = time.Time{}
}

func (m shellModel) View() tea.View {
	header := renderHeader(m.state)
	headerBlock := header
	if header != "" {
		headerBlock = "\n" + header
	}
	if m.state.startupActive {
		return tea.NewView(headerBlock + "\n" + renderStartupLine(m))
	}
	outputLines := m.state.output
	headerLines := 0
	if header != "" {
		headerLines = countLines(headerBlock)
	}
	availableHeight := m.height - headerLines - 1
	if availableHeight < 0 {
		availableHeight = 0
	}
	if len(outputLines) > availableHeight {
		outputLines = outputLines[len(outputLines)-availableHeight:]
	}
	body := strings.Join(outputLines, "\n")
	if body == "" {
		if m.busy {
			if header != "" {
				return tea.NewView(headerBlock + "\n" + renderLoadingLine(m))
			}
			return tea.NewView(renderLoadingLine(m))
		}
		lines := []string{""}
		if header != "" {
			lines = []string{"", header, ""}
		}
		lines = append(lines, renderPrompt(m))
		return tea.NewView(strings.Join(lines, "\n"))
	}
	separator := "\n"
	if len(outputLines) > 0 && isPromptLine(outputLines[0]) {
		// After a clear, keep a blank line between the header and first prompt line.
		separator = "\n\n"
	}
	if m.busy {
		// Preserve the same header spacing even while the spinner is visible.
		if header != "" {
			return tea.NewView(headerBlock + separator + body + "\n" + renderLoadingLine(m))
		}
		return tea.NewView(body + "\n" + renderLoadingLine(m))
	}
	lines := []string{body}
	if header != "" {
		lines = []string{"", header, body}
		if separator == "\n\n" {
			lines = []string{"", header, "", body}
		}
	} else if separator == "\n\n" {
		lines = []string{"", body}
	}
	lastLine := ""
	if len(outputLines) > 0 {
		lastLine = outputLines[len(outputLines)-1]
	}
	if len(outputLines) == 0 || (lastLine != "" && !isPromptLine(lastLine)) {
		// Keep a blank line before the prompt unless we just printed a prompt line.
		lines = append(lines, "")
	}
	lines = append(lines, renderPrompt(m))
	return tea.NewView(strings.Join(lines, "\n"))
}

func (m shellModel) Cursor() *tea.Cursor {
	if m.state != nil && m.state.promptDialog != nil {
		return m.state.promptDialog.Cursor()
	}
	return nil
}

func (m *shellModel) appendCommandLine(line string) {
	prefix := renderPromptPrefix(m.state)
	if len(m.state.output) > 0 {
		lastLine := m.state.output[len(m.state.output)-1]
		if lastLine != "" && !isPromptLine(lastLine) {
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
		return "Create your first app: vibe myapp"
	}
	return fmt.Sprintf("Create your first app: %s %s", theme.BannerVerb.Render("vibe"), theme.BannerApp.Render("myapp"))
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
	if m.state != nil && m.state.promptDialog != nil {
		return m.state.promptDialog.View()
	}
	if m.state != nil && m.state.confirmDeleteApp != "" {
		return confirmDeletePrompt(m.state) + m.input.View()
	}
	return renderPromptPrefix(m.state) + m.input.View() + " "
}

func renderLoadingLine(m shellModel) string {
	label := ""
	if m.state != nil {
		label = strings.TrimSpace(m.state.busyLabel)
	}
	if label == "" {
		return m.promptSpin.View()
	}
	theme := shellTheme()
	if !theme.Enabled {
		return fmt.Sprintf("%s %s", m.promptSpin.View(), label)
	}
	return fmt.Sprintf("%s %s", theme.StatusConnecting.Render(m.promptSpin.View()), theme.Muted.Render(label))
}

func renderStartupLine(m shellModel) string {
	stage := strings.TrimSpace(m.state.startupStage)
	if stage == "" {
		stage = "Working..."
	}
	theme := shellTheme()
	if !theme.Enabled {
		return fmt.Sprintf("%s %s", m.promptSpin.View(), stage)
	}
	return fmt.Sprintf("%s %s", theme.StatusConnecting.Render(m.promptSpin.View()), theme.Muted.Render(stage))
}

func hostLabel(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "your server"
	}
	return host
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

type dialogResultProvider interface {
	Result() (*dialogs.Result, bool)
}

type dialogValuesProvider interface {
	Values() (map[string]string, bool)
}

func dialogResult(dialog dialogs.Dialog) (*dialogs.Result, bool) {
	if provider, ok := dialog.(dialogResultProvider); ok {
		if result, ok := provider.Result(); ok {
			return result, true
		}
		return nil, false
	}
	if provider, ok := dialog.(dialogValuesProvider); ok {
		if values, ok := provider.Values(); ok {
			return &dialogs.Result{Values: values}, true
		}
	}
	return nil, false
}

func isPromptLine(line string) bool {
	stripped := strings.TrimSpace(ansi.Strip(line))
	return strings.HasPrefix(stripped, "viberun ")
}
