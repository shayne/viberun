// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shayne/viberun/internal/ui/components"
)

// ShellModel is the v2 shell REPL model.
type ShellModel struct {
	output     []string
	input      string
	history    []string
	historyIdx int
	quit       bool
	scope      shellScope
	app        string
	backend    Backend
}

// NewShellModel constructs a new shell model.
func NewShellModel() ShellModel {
	return ShellModel{scope: scopeGlobal}
}

// Init initializes the model.
func (m ShellModel) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages.
func (m ShellModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if _, ok := msg.(tea.KeyPressMsg); !ok {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+l":
			m.output = nil
		case "ctrl+c":
			m.input = ""
		case "ctrl+d":
			if strings.TrimSpace(m.input) == "" {
				m.quit = true
				return m, tea.Quit
			}
		case "up":
			m.historyPrev()
		case "down":
			m.historyNext()
		case "enter":
			line := strings.TrimSpace(m.input)
			m.input = ""
			if line == "" {
				m.appendCommandLine("")
				return m, nil
			}
			m.appendCommandLine(line)
			m.history = append(m.history, line)
			m.historyIdx = len(m.history)
			if cmd, err := parseShellCommand(line); err == nil {
				if !m.handleParsedCommand(cmd) && m.backend != nil {
					out, cmd := m.backend.Dispatch(line)
					if out != "" {
						m.output = append(m.output, out)
					}
					if cmd != nil {
						return m, cmd
					}
				}
			} else {
				m.output = append(m.output, "error: "+err.Error())
			}
		}
		key := msg.Key()
		if key.Code == tea.KeyBackspace {
			m.input = trimLastRune(m.input)
			return m, nil
		}
		if key.Text != "" && key.Mod == 0 {
			m.input += key.Text
			return m, nil
		}
	}
	return m, nil
}

// View renders the model.
func (m ShellModel) View() tea.View {
	if len(m.output) == 0 {
		return tea.NewView(m.promptLine())
	}
	body := strings.Join(m.output, "\n")
	lines := []string{body}
	lastLine := m.output[len(m.output)-1]
	if lastLine != "" && !isPromptLine(lastLine) {
		lines = append(lines, "")
	}
	lines = append(lines, m.promptLine())
	return tea.NewView(strings.Join(lines, "\n"))
}

func (m *ShellModel) historyPrev() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIdx <= 0 {
		m.historyIdx = 0
		m.input = m.history[0]
		return
	}
	m.historyIdx--
	if m.historyIdx < 0 {
		m.historyIdx = 0
	}
	m.input = m.history[m.historyIdx]
}

func (m *ShellModel) historyNext() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIdx >= len(m.history)-1 {
		m.historyIdx = len(m.history)
		m.input = ""
		return
	}
	m.historyIdx++
	if m.historyIdx >= len(m.history) {
		m.historyIdx = len(m.history)
		m.input = ""
		return
	}
	m.input = m.history[m.historyIdx]
}

func (m *ShellModel) handleParsedCommand(cmd parsedCommand) bool {
	switch cmd.name {
	case "help", "?":
		if len(cmd.args) > 0 {
			if isHelpAll(cmd.args[0]) {
				m.output = append(m.output, renderHelpGlobal(true))
				return true
			}
			m.output = append(m.output, renderCommandHelp(cmd.args[0], scopeGlobal))
			return true
		}
		m.output = append(m.output, renderHelpGlobal(false))
		return true
	case "setup":
		m.output = append(m.output, renderSetupIntro())
		return true
	default:
		// TODO: wire full command dispatch once backend is ported.
	}
	return false
}

func (m *ShellModel) appendCommandLine(line string) {
	if len(m.output) > 0 {
		lastLine := m.output[len(m.output)-1]
		if lastLine != "" && !isPromptLine(lastLine) {
			m.output = append(m.output, "")
		}
	}
	m.output = append(m.output, m.promptPrefix()+line)
}

func (m ShellModel) promptPrefix() string {
	return components.PromptPrefix(m.app, m.scope == scopeAppConfig)
}

func (m ShellModel) promptLine() string {
	return components.PromptLine(m.app, m.scope == scopeAppConfig, m.input)
}

func isHelpAll(arg string) bool {
	switch strings.TrimSpace(strings.ToLower(arg)) {
	case "--all", "-a", "all":
		return true
	default:
		return false
	}
}

func isPromptLine(line string) bool {
	stripped := strings.TrimSpace(line)
	return strings.HasPrefix(stripped, "viberun ")
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}
