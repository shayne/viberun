// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shayne/viberun/internal/target"
)

type appSnapshot struct {
	Name string
	Port int
}

func startupConnectCmd(state *shellState) tea.Cmd {
	host := strings.TrimSpace(state.host)
	cfg := state.cfg
	agent := strings.TrimSpace(state.agent)
	return func() tea.Msg {
		if host == "" {
			return startupConnectMsg{reachable: false, bootstrapped: false, err: errors.New("no host configured")}
		}
		resolved, err := target.ResolveHost(host, cfg)
		if err != nil {
			return startupConnectMsg{reachable: false, bootstrapped: false, err: err}
		}
		gateway, err := startGateway(resolved.Host, agent, devChannelEnv(), false)
		if err != nil {
			if isMissingServerError(err) {
				return startupConnectMsg{reachable: true, bootstrapped: false, gatewayHost: resolved.Host}
			}
			return startupConnectMsg{reachable: false, bootstrapped: false, err: err}
		}
		return startupConnectMsg{reachable: true, bootstrapped: true, gateway: gateway, gatewayHost: resolved.Host}
	}
}

func startupAppsCmd(state *shellState) tea.Cmd {
	return func() tea.Msg {
		if state == nil || state.gateway == nil {
			return startupAppsMsg{err: errors.New("gateway not connected")}
		}
		apps, err := runRemoteAppsList(state.gateway)
		if err != nil {
			return startupAppsMsg{err: err}
		}
		proxyEnabled := false
		if len(apps) > 0 {
			if summary, err := fetchRemoteProxyConfig(state.gateway); err == nil {
				proxyEnabled = summary.Enabled
			}
		}
		return startupAppsMsg{apps: appSnapshotsFromNames(apps), proxyEnabled: proxyEnabled}
	}
}

func startupForwardCmd(state *shellState, apps []appSnapshot, proxyEnabled bool) tea.Cmd {
	gateway := state.gateway
	return func() tea.Msg {
		if gateway == nil {
			return startupReadyMsg{err: errors.New("gateway not connected")}
		}
		summaries, err := buildAppSummaries(gateway, apps, proxyEnabled)
		if err != nil {
			return startupReadyMsg{err: err}
		}
		return startupReadyMsg{apps: summaries}
	}
}

func appSnapshotsFromNames(apps []string) []appSnapshot {
	out := make([]appSnapshot, 0, len(apps))
	for _, name := range apps {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, appSnapshot{Name: name})
	}
	return out
}

func buildAppSummaries(gateway *gatewayClient, apps []appSnapshot, proxyEnabled bool) ([]appSummary, error) {
	if gateway == nil {
		return nil, errors.New("gateway not connected")
	}
	results := make([]appSummary, 0, len(apps))
	for _, app := range apps {
		name := strings.TrimSpace(app.Name)
		if name == "" {
			continue
		}
		port := app.Port
		if port <= 0 {
			resolvedPort, err := resolveHostPort(gateway, name, "")
			if err != nil {
				results = append(results, appSummary{Name: name, Status: appStatusUnknown, ForwardErr: forwardErrorMessage(err)})
				continue
			}
			port = resolvedPort
		}
		localURL := ""
		if port > 0 {
			localURL = fmt.Sprintf("http://localhost:%d", port)
		}
		status := appStatusUnknown
		if fetched, err := fetchAppStatus(gateway, name); err == nil {
			status = fetched
		}
		publicURL := ""
		if proxyEnabled {
			if info, err := fetchProxyInfo(gateway, name); err == nil {
				if !info.Disabled && strings.TrimSpace(info.URL) != "" {
					publicURL = strings.TrimSpace(info.URL)
				}
			}
		}
		results = append(results, appSummary{
			Name:      name,
			Status:    status,
			LocalURL:  localURL,
			PublicURL: publicURL,
			Port:      port,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

func applyAppSummaries(state *shellState, summaries []appSummary) []appSummary {
	if state == nil {
		return summaries
	}
	updated := make([]appSummary, 0, len(summaries))
	for _, summary := range summaries {
		name := strings.TrimSpace(summary.Name)
		if name == "" {
			continue
		}
		updated = append(updated, applyForwardStatus(state, summary))
	}
	return updated
}

func applyForwardStatus(state *shellState, summary appSummary) appSummary {
	if state == nil {
		return summary
	}
	if isLocalHost(state.gatewayHost) {
		summary.Forwarded = summary.LocalURL != ""
		summary.ForwardErr = ""
		return summary
	}
	if summary.Port <= 0 || summary.LocalURL == "" {
		if summary.ForwardErr == "" {
			summary.ForwardErr = "unavailable"
		}
		return summary
	}
	if state.forwarder != nil {
		if ok, err, found := state.forwarder.status(summary.Name, summary.Port); found {
			if ok {
				summary.Forwarded = true
				summary.ForwardErr = ""
				return summary
			}
			if summary.ForwardErr == "" {
				summary.ForwardErr = forwardErrorMessage(err)
			}
			return summary
		}
	}
	if summary.ForwardErr == "" {
		summary.ForwardErr = "unavailable"
	}
	return summary
}

func forwardErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "address already in use") {
		return "local port busy"
	}
	return "unavailable"
}

func renderStartupSummary(state *shellState) string {
	if state == nil {
		return ""
	}
	if len(state.apps) == 0 {
		return renderWelcomeSummary(state)
	}
	return renderAppsTable(state, true)
}

func renderWelcomeSummary(state *shellState) string {
	theme := shellTheme()
	verb := "vibe"
	app := "<name>"
	if theme.Enabled {
		verb = theme.BannerVerb.Render("vibe")
		app = theme.BannerApp.Render("<name>")
	}
	lines := []string{
		"",
		"Welcome to viberun.",
		fmt.Sprintf("Create your first app with: %s %s", verb, app),
		"",
		renderStartupTip(theme),
	}
	return strings.Join(lines, "\n")
}

func renderStartupTip(theme ShellTheme) string {
	if !theme.Enabled {
		return "Tip: vibe <app> to jump in, or open <app> to view it."
	}
	vibe := theme.BannerVerb.Render("vibe")
	open := theme.BannerVerb.Render("open")
	app := theme.BannerApp.Render("<app>")
	return fmt.Sprintf("Tip: %s %s to jump in, or %s %s to view it.", vibe, app, open, app)
}

func renderAppsTable(state *shellState, includeTip bool) string {
	if state == nil {
		return ""
	}
	if len(state.apps) == 0 {
		return "No apps found."
	}
	rows := make([]appRow, 0, len(state.apps))
	for _, app := range state.apps {
		rows = append(rows, formatAppRow(app))
	}
	nameWidth, statusWidth, localWidth := columnWidths(rows)
	publicWidth := 0
	for _, row := range rows {
		if len(row.public) > publicWidth {
			publicWidth = len(row.public)
		}
	}
	header := fmt.Sprintf("Apps on %s", hostLabel(state.host))
	divider := strings.Repeat("-", len(header))
	headerRow := renderAppRowHeader(nameWidth, statusWidth, localWidth, publicWidth)
	theme := shellTheme()
	if theme.Enabled {
		header = theme.HelpHeader.Render(header)
		divider = theme.Muted.Render(divider)
		headerRow = theme.Muted.Render(headerRow)
	}
	lines := []string{"", header}
	lines = append(lines, divider)
	lines = append(lines, headerRow)
	for _, row := range rows {
		lines = append(lines, renderAppRow(row, nameWidth, statusWidth, localWidth, publicWidth))
	}
	if includeTip {
		lines = append(lines, "", renderStartupTip(shellTheme()))
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

type appRow struct {
	name       string
	status     string
	statusKind appStatus
	local      string
	public     string
	localErr   string
}

func formatAppRow(app appSummary) appRow {
	local := app.LocalURL
	localErr := strings.TrimSpace(app.ForwardErr)
	if localErr != "" {
		local = localErr
	}
	if local == "" {
		local = "-"
	}
	public := strings.TrimSpace(app.PublicURL)
	if public == "" {
		public = "-"
	}
	return appRow{
		name:       app.Name,
		status:     string(app.Status),
		statusKind: app.Status,
		local:      local,
		public:     public,
		localErr:   localErr,
	}
}

func columnWidths(rows []appRow) (int, int, int) {
	nameWidth := len("app")
	statusWidth := len("status")
	localWidth := len("local")
	for _, row := range rows {
		if len(row.name) > nameWidth {
			nameWidth = len(row.name)
		}
		if len(row.status) > statusWidth {
			statusWidth = len(row.status)
		}
		if len(row.local) > localWidth {
			localWidth = len(row.local)
		}
	}
	return nameWidth, statusWidth, localWidth
}

func renderAppRowHeader(nameWidth, statusWidth, localWidth, publicWidth int) string {
	parts := []string{
		padColumn("app", nameWidth),
		padColumn("status", statusWidth),
		padColumn("local", localWidth),
		padColumn("public", publicWidth),
	}
	return strings.Join(parts, "  ")
}

func renderAppRow(row appRow, nameWidth, statusWidth, localWidth, publicWidth int) string {
	theme := shellTheme()
	name := padColumn(row.name, nameWidth)
	status := padColumn(row.status, statusWidth)
	local := padColumn(row.local, localWidth)
	public := padColumn(row.public, publicWidth)
	if theme.Enabled {
		name = theme.Value.Render(name)
		statusStyle := theme.StatusUnknown
		switch row.statusKind {
		case appStatusRunning:
			statusStyle = theme.StatusRunning
		case appStatusStopped:
			statusStyle = theme.StatusStopped
		case appStatusUnknown:
			statusStyle = theme.StatusUnknown
		}
		if row.localErr != "" {
			local = theme.StatusUnavailable.Render(local)
		} else if row.local != "-" {
			local = theme.Link.Render(local)
		} else {
			local = theme.Muted.Render(local)
		}
		status = statusStyle.Render(status)
		if row.public != "-" {
			public = theme.Link.Render(public)
		} else {
			public = theme.Muted.Render(public)
		}
	}
	parts := []string{name, status, local, public}
	return strings.Join(parts, "  ")
}

func padColumn(value string, width int) string {
	if width <= len(value) {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}
