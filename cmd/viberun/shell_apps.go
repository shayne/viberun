// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"strings"
)

func findAppSummary(state *shellState, app string) (appSummary, bool) {
	if state == nil {
		return appSummary{}, false
	}
	app = strings.TrimSpace(app)
	if app == "" {
		return appSummary{}, false
	}
	for _, summary := range state.apps {
		if summary.Name == app {
			return summary, true
		}
	}
	return appSummary{}, false
}

func updateAppSummary(state *shellState, summary appSummary) {
	if state == nil {
		return
	}
	for i := range state.apps {
		if state.apps[i].Name == summary.Name {
			state.apps[i] = summary
			return
		}
	}
	state.apps = append(state.apps, summary)
}

func removeAppSummary(state *shellState, app string) {
	if state == nil {
		return
	}
	app = strings.TrimSpace(app)
	if app == "" {
		return
	}
	for i, summary := range state.apps {
		if summary.Name == app {
			state.apps = append(state.apps[:i], state.apps[i+1:]...)
			return
		}
	}
}

func markAppsStale(state *shellState) {
	if state == nil {
		return
	}
	state.appsLoaded = false
	state.appsSyncing = false
	state.apps = nil
}

func fetchAppStatus(gateway *gatewayClient, app string) (appStatus, error) {
	if gateway == nil {
		return appStatusUnknown, errors.New("gateway not connected")
	}
	remoteArgs := buildAppCommandArgs("", app, []string{"status"})
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return appStatusUnknown, err
	}
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "running":
		return appStatusRunning, nil
	case "stopped", "missing":
		return appStatusStopped, nil
	default:
		return appStatusUnknown, nil
	}
}

func openAppURL(state *shellState, app string) (string, error) {
	if state == nil {
		return "", errors.New("gateway not connected")
	}
	summary, ok := findAppSummary(state, app)
	if !ok {
		if state.gateway == nil {
			return "", errors.New("gateway not connected")
		}
		proxyEnabled := false
		if info, err := fetchRemoteProxyConfig(state.gateway); err == nil {
			proxyEnabled = info.Enabled
		}
		summaries, err := buildAppSummaries(state.gateway, []appSnapshot{{Name: app}}, proxyEnabled)
		if err != nil || len(summaries) == 0 {
			if err == nil {
				err = errors.New("app not found")
			}
			return "", err
		}
		summary = summaries[0]
	}
	summary = applyForwardStatus(state, summary)
	if ok {
		updateAppSummary(state, summary)
	}
	url := ""
	if strings.TrimSpace(summary.PublicURL) != "" {
		url = summary.PublicURL
	} else if strings.TrimSpace(summary.LocalURL) != "" && (summary.Forwarded || isLocalHost(state.gatewayHost)) {
		url = summary.LocalURL
	}
	if url == "" {
		return "", fmt.Errorf("URL is not available")
	}
	if err := openURL(url); err != nil {
		return "", err
	}
	return "", nil
}
