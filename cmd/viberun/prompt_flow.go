// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/shayne/viberun/internal/proxy"
	"github.com/shayne/viberun/internal/tui/dialogs"
)

type promptFlow interface {
	Dialog() dialogs.Dialog
	ApplyResult(dialogs.Result)
	Done() bool
	Cancelled() bool
}

type confirmFlow struct {
	dialog    *dialogs.ConfirmDialog
	done      bool
	cancelled bool
	confirmed bool
}

func newConfirmFlow(id, title, description string, defaultYes bool) *confirmFlow {
	return &confirmFlow{dialog: dialogs.NewConfirmDialog(id, title, description, defaultYes)}
}

func (f *confirmFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *confirmFlow) ApplyResult(result dialogs.Result) {
	f.done = true
	if result.Cancelled {
		f.cancelled = true
		return
	}
	f.confirmed = result.Confirmed
}

func (f *confirmFlow) Done() bool {
	return f.done
}

func (f *confirmFlow) Cancelled() bool {
	return f.cancelled
}

func (f *confirmFlow) Confirmed() bool {
	return f.confirmed
}

type setupPlan struct {
	Host            string
	UpdateArtifacts bool
	Wipe            *wipePlan
}

type proxyPlan struct {
	Host       string
	Domain     string
	PublicIP   string
	Username   string
	Password   string
	EditConfig bool
	Configured bool
}

type wipePlan struct {
	Host      string
	WipeLocal bool
}

type passwordPlan struct {
	Host     string
	Username string
	Password string
	Action   string
}

type setupFlowInput struct {
	ExistingHost     string
	AlreadyConnected bool
	DevMode          bool
}

type setupFlowStep int

const (
	setupStepRerunConfirm setupFlowStep = iota
	setupStepWipeConfirm
	setupStepWipeToken
	setupStepHost
	setupStepUpdateArtifacts
)

type setupFlow struct {
	input     setupFlowInput
	step      setupFlowStep
	dialog    dialogs.Dialog
	done      bool
	cancelled bool
	plan      *setupPlan
	note      string
}

func newSetupFlow(input setupFlowInput) *setupFlow {
	flow := &setupFlow{input: input}
	if input.AlreadyConnected {
		title := "You're already connected."
		if host := strings.TrimSpace(input.ExistingHost); host != "" {
			title = "You're already connected to " + host + "."
		}
		flow.step = setupStepRerunConfirm
		flow.dialog = dialogs.NewConfirmDialog("setup-rerun", title, "Set up a different server?", false)
		return flow
	}
	flow.step = setupStepHost
	flow.dialog = setupHostDialog(input)
	return flow
}

func (f *setupFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *setupFlow) ApplyResult(result dialogs.Result) {
	if f.done {
		return
	}
	if result.Cancelled {
		f.cancelled = true
		f.done = true
		return
	}
	switch f.step {
	case setupStepRerunConfirm:
		if !result.Confirmed {
			host := strings.TrimSpace(f.input.ExistingHost)
			if host != "" {
				f.note = fmt.Sprintf("Okay, staying connected to %s.", host)
			} else {
				f.note = "Okay, staying connected."
			}
			f.done = true
			return
		}
		f.step = setupStepWipeConfirm
		f.dialog = newWipeConfirmDialog(f.input.ExistingHost)
		return
	case setupStepWipeConfirm:
		if result.Confirmed {
			f.step = setupStepWipeToken
			f.dialog = newWipeTokenDialog()
			return
		}
		f.step = setupStepHost
		f.dialog = setupHostDialog(f.input)
		return
	case setupStepWipeToken:
		if f.plan == nil {
			f.plan = &setupPlan{}
		}
		f.plan.Wipe = &wipePlan{Host: strings.TrimSpace(f.input.ExistingHost), WipeLocal: false}
		f.step = setupStepHost
		f.dialog = setupHostDialog(f.input)
		return
	case setupStepHost:
		host := strings.TrimSpace(result.Values["host"])
		if host == "" {
			f.cancelled = true
			f.done = true
			return
		}
		if f.plan == nil {
			f.plan = &setupPlan{}
		}
		f.plan.Host = host
		prompt, defaultYes, shouldPrompt, updateArtifacts := setupUpdatePrompt(host, f.input.DevMode)
		if !shouldPrompt {
			f.plan.UpdateArtifacts = updateArtifacts
			f.done = true
			return
		}
		f.step = setupStepUpdateArtifacts
		f.dialog = dialogs.NewConfirmDialog("setup-update", prompt, "", defaultYes)
		return
	case setupStepUpdateArtifacts:
		if f.plan == nil {
			f.plan = &setupPlan{}
		}
		f.plan.UpdateArtifacts = result.Confirmed
		f.done = true
		return
	}
}

func (f *setupFlow) Done() bool {
	return f.done
}

func (f *setupFlow) Cancelled() bool {
	return f.cancelled
}

func (f *setupFlow) Plan() *setupPlan {
	return f.plan
}

func (f *setupFlow) Note() string {
	return f.note
}

func setupHostDialog(input setupFlowInput) *dialogs.FormDialog {
	defaultHost := ""
	if input.AlreadyConnected {
		defaultHost = strings.TrimSpace(input.ExistingHost)
	}
	fields := []dialogs.Field{{
		ID:          "host",
		Title:       "Server login",
		Placeholder: "user@host  # username + address",
		Required:    true,
		Default:     defaultHost,
		Validate: func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("please enter a server login")
			}
			return nil
		},
	}}
	return dialogs.NewFormDialog("setup-host", "Server login", "", fields)
}

func setupUpdatePrompt(host string, devMode bool) (string, bool, bool, bool) {
	bootstrapped, _ := checkHostBootstrapped(host)
	if !bootstrapped {
		return "", false, false, true
	}
	if devMode {
		return "Update server binary and images?", true, true, true
	}
	remoteVersion, err := fetchRemoteServerVersion(host)
	if isDevChannel() {
		if err != nil {
			return "Server version unknown. Update server now?", true, true, true
		}
		if update, defaultYes := devUpdateDecision(version, remoteVersion); update {
			prompt := fmt.Sprintf("Server is %s. Update to %s?", strings.TrimSpace(remoteVersion), strings.TrimSpace(version))
			return prompt, defaultYes, true, true
		}
		return "", false, false, false
	}
	if err != nil {
		return "Server version unknown. Update server now?", false, true, true
	}
	if cmp, ok := compareSemver(version, remoteVersion); !ok {
		return "Server version unknown. Update server now?", false, true, true
	} else if cmp > 0 {
		prompt := fmt.Sprintf("Server is %s. Update to %s?", strings.TrimSpace(remoteVersion), strings.TrimSpace(version))
		return prompt, true, true, true
	}
	return "", false, false, false
}

type wipeFlowInput struct {
	Host      string
	WipeLocal bool
}

type wipeFlowStep int

const (
	wipeStepConfirm wipeFlowStep = iota
	wipeStepToken
)

type wipeFlow struct {
	input     wipeFlowInput
	step      wipeFlowStep
	dialog    dialogs.Dialog
	done      bool
	cancelled bool
	plan      *wipePlan
	note      string
}

func newWipeFlow(input wipeFlowInput) *wipeFlow {
	flow := &wipeFlow{input: input, step: wipeStepConfirm}
	flow.dialog = newWipeConfirmDialog(input.Host)
	return flow
}

func (f *wipeFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *wipeFlow) ApplyResult(result dialogs.Result) {
	if f.done {
		return
	}
	if result.Cancelled {
		f.cancelled = true
		f.done = true
		return
	}
	switch f.step {
	case wipeStepConfirm:
		if !result.Confirmed {
			f.note = "Wipe cancelled."
			f.done = true
			return
		}
		f.step = wipeStepToken
		f.dialog = newWipeTokenDialog()
	case wipeStepToken:
		f.plan = &wipePlan{
			Host:      strings.TrimSpace(f.input.Host),
			WipeLocal: f.input.WipeLocal,
		}
		f.done = true
	}
}

func (f *wipeFlow) Done() bool {
	return f.done
}

func (f *wipeFlow) Cancelled() bool {
	return f.cancelled
}

func (f *wipeFlow) Plan() *wipePlan {
	return f.plan
}

func (f *wipeFlow) Note() string {
	return f.note
}

func newWipeConfirmDialog(host string) *dialogs.ConfirmDialog {
	title := "Wipe this server?"
	host = strings.TrimSpace(host)
	if host != "" {
		title = "Wipe " + host + "?"
	}
	return dialogs.NewConfirmDialog("wipe-confirm", title, "This removes all viberun data, containers, and configuration from that server.", false)
}

func newWipeTokenDialog() *dialogs.FormDialog {
	field := dialogs.Field{
		ID:          "confirm",
		Title:       "Type WIPE to confirm",
		Description: "This permanently removes all viberun data from the server. This cannot be undone.",
		Placeholder: "WIPE",
		Required:    true,
		Validate: func(value string) error {
			if strings.TrimSpace(value) != "WIPE" {
				return errors.New("type WIPE to confirm")
			}
			return nil
		},
	}
	return dialogs.NewFormDialog("wipe-token", "Type WIPE to confirm", "", []dialogs.Field{field})
}

type passwordFlowInput struct {
	Host     string
	Username string
	Action   string
}

type passwordFlow struct {
	input     passwordFlowInput
	dialog    dialogs.Dialog
	done      bool
	cancelled bool
	plan      *passwordPlan
}

func newPasswordFlow(input passwordFlowInput) *passwordFlow {
	field := dialogs.Field{
		ID:       "password",
		Title:    "Password",
		Secret:   true,
		Required: true,
		Validate: func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("password is required")
			}
			return nil
		},
	}
	return &passwordFlow{
		input:  input,
		dialog: dialogs.NewFormDialog("password", "Password", "", []dialogs.Field{field}),
	}
}

func (f *passwordFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *passwordFlow) ApplyResult(result dialogs.Result) {
	if f.done {
		return
	}
	if result.Cancelled {
		f.cancelled = true
		f.done = true
		return
	}
	password := strings.TrimSpace(result.Values["password"])
	f.plan = &passwordPlan{
		Host:     strings.TrimSpace(f.input.Host),
		Username: strings.TrimSpace(f.input.Username),
		Password: password,
		Action:   strings.TrimSpace(f.input.Action),
	}
	f.done = true
}

func (f *passwordFlow) Done() bool {
	return f.done
}

func (f *passwordFlow) Cancelled() bool {
	return f.cancelled
}

func (f *passwordFlow) Plan() *passwordPlan {
	return f.plan
}

type proxyFlowInput struct {
	Host            string
	Summary         proxyConfigSummary
	Configured      bool
	ConfigError     bool
	PublicIP        string
	UpdateArtifacts bool
	ForceSetup      bool
	ShowSkipHint    bool
}

type proxyFlowStep int

const (
	proxyStepConfigErrorConfirm proxyFlowStep = iota
	proxyStepSetupConfirm
	proxyStepEditConfirm
	proxyStepChangeDomainConfirm
	proxyStepDomainInput
	proxyStepChangeIPConfirm
	proxyStepIPInput
	proxyStepChangeUserConfirm
	proxyStepAuthInput
	proxyStepFinalize
)

type proxySetupFlow struct {
	input      proxyFlowInput
	step       proxyFlowStep
	dialog     dialogs.Dialog
	done       bool
	cancelled  bool
	plan       *proxyPlan
	note       string
	configured bool
	editConfig bool
	domain     string
	publicIP   string
	username   string
	password   string
}

func newProxySetupFlow(input proxyFlowInput) *proxySetupFlow {
	flow := &proxySetupFlow{
		input:      input,
		configured: input.Configured,
		domain:     strings.TrimSpace(input.Summary.BaseDomain),
		publicIP:   strings.TrimSpace(input.Summary.PublicIP),
		username:   strings.TrimSpace(input.Summary.PrimaryUser),
	}
	if flow.publicIP == "" {
		flow.publicIP = strings.TrimSpace(input.PublicIP)
	}
	switch {
	case input.ConfigError && !input.ForceSetup:
		flow.step = proxyStepConfigErrorConfirm
		flow.dialog = dialogs.NewConfirmDialog("proxy-config-error", "Existing public domain settings could not be read. Continue with setup?", "", false)
	case input.Configured:
		flow.step = proxyStepEditConfirm
		flow.dialog = dialogs.NewConfirmDialog("proxy-edit", "Edit public domain settings?", proxySummaryDescription(input.Summary), false)
	case input.ForceSetup:
		flow.step = proxyStepDomainInput
		flow.dialog = newProxyDomainDialog("", false)
	default:
		flow.step = proxyStepSetupConfirm
		flow.dialog = dialogs.NewConfirmDialog("proxy-setup", "Set up a public domain name?", "", false)
	}
	return flow
}

func (f *proxySetupFlow) Dialog() dialogs.Dialog {
	return f.dialog
}

func (f *proxySetupFlow) ApplyResult(result dialogs.Result) {
	if f.done {
		return
	}
	if result.Cancelled {
		f.cancelled = true
		f.done = true
		return
	}
	switch f.step {
	case proxyStepConfigErrorConfirm:
		if !result.Confirmed {
			f.note = proxySkipHint(f.input.ShowSkipHint)
			f.done = true
			return
		}
		f.configured = false
		f.step = proxyStepDomainInput
		f.dialog = newProxyDomainDialog("", false)
	case proxyStepSetupConfirm:
		if !result.Confirmed {
			f.note = proxySkipHint(f.input.ShowSkipHint)
			f.done = true
			return
		}
		f.step = proxyStepDomainInput
		f.dialog = newProxyDomainDialog("", false)
	case proxyStepEditConfirm:
		f.editConfig = result.Confirmed
		if !f.editConfig && !f.input.UpdateArtifacts {
			f.done = true
			return
		}
		if !f.editConfig {
			f.step = proxyStepFinalize
			f.finalizePlan()
			return
		}
		f.step = proxyStepChangeDomainConfirm
		f.dialog = dialogs.NewConfirmDialog("proxy-domain-change", "Change public domain?", "", false)
	case proxyStepChangeDomainConfirm:
		if result.Confirmed {
			f.step = proxyStepDomainInput
			f.dialog = newProxyDomainDialog(f.domain, true)
			return
		}
		f.step = proxyStepChangeIPConfirm
		f.dialog = dialogs.NewConfirmDialog("proxy-ip-change", "Change public IP?", "", false)
	case proxyStepDomainInput:
		domain := strings.TrimSpace(result.Values["domain"])
		if normalized, err := proxy.NormalizeDomainSuffix(domain); err == nil {
			domain = normalized
		}
		f.domain = domain
		if f.configured || f.editConfig {
			f.step = proxyStepChangeIPConfirm
			f.dialog = dialogs.NewConfirmDialog("proxy-ip-change", "Change public IP?", "", false)
		} else {
			f.step = proxyStepAuthInput
			f.dialog = newProxyAuthDialog(f.username, true)
		}
	case proxyStepChangeIPConfirm:
		if result.Confirmed {
			f.step = proxyStepIPInput
			f.dialog = newProxyIPDialog(f.publicIP)
			return
		}
		f.step = proxyStepChangeUserConfirm
		f.dialog = dialogs.NewConfirmDialog("proxy-user-change", "Change primary user?", "", false)
	case proxyStepIPInput:
		f.publicIP = strings.TrimSpace(result.Values["ip"])
		f.step = proxyStepChangeUserConfirm
		f.dialog = dialogs.NewConfirmDialog("proxy-user-change", "Change primary user?", "", false)
	case proxyStepChangeUserConfirm:
		if result.Confirmed {
			f.step = proxyStepAuthInput
			f.dialog = newProxyAuthDialog(f.username, true)
			return
		}
		f.step = proxyStepFinalize
		f.finalizePlan()
	case proxyStepAuthInput:
		f.username = strings.TrimSpace(result.Values["username"])
		f.password = strings.TrimSpace(result.Values["password"])
		f.step = proxyStepFinalize
		f.finalizePlan()
	}
}

func (f *proxySetupFlow) finalizePlan() {
	if f.done {
		return
	}
	if f.publicIP == "" {
		f.publicIP = strings.TrimSpace(f.input.PublicIP)
	}
	f.plan = &proxyPlan{
		Host:       strings.TrimSpace(f.input.Host),
		Domain:     strings.TrimSpace(f.domain),
		PublicIP:   strings.TrimSpace(f.publicIP),
		Username:   strings.TrimSpace(f.username),
		Password:   strings.TrimSpace(f.password),
		EditConfig: f.editConfig,
		Configured: f.configured,
	}
	f.done = true
}

func (f *proxySetupFlow) Done() bool {
	return f.done
}

func (f *proxySetupFlow) Cancelled() bool {
	return f.cancelled
}

func (f *proxySetupFlow) Plan() *proxyPlan {
	return f.plan
}

func (f *proxySetupFlow) Note() string {
	return f.note
}

func proxySkipHint(show bool) string {
	if !show {
		return ""
	}
	return "Set up a public domain name later with: proxy setup"
}

func proxySummaryDescription(summary proxyConfigSummary) string {
	lines := []string{}
	if strings.TrimSpace(summary.BaseDomain) != "" {
		lines = append(lines, "Domain: "+strings.TrimSpace(summary.BaseDomain))
	}
	if strings.TrimSpace(summary.PublicIP) != "" {
		lines = append(lines, "Public IP: "+strings.TrimSpace(summary.PublicIP))
	}
	if strings.TrimSpace(summary.PrimaryUser) != "" {
		lines = append(lines, "Primary user: "+strings.TrimSpace(summary.PrimaryUser))
	}
	return strings.Join(lines, "\n")
}

func newProxyDomainDialog(defaultDomain string, withDefault bool) *dialogs.FormDialog {
	description := "Your apps will be available at myapp.<domain>"
	field := dialogs.Field{
		ID:          "domain",
		Title:       "Public domain name",
		Description: description,
		Placeholder: "mydomain.com",
		Required:    true,
		Validate: func(value string) error {
			_, err := proxy.NormalizeDomainSuffix(value)
			return err
		},
	}
	if withDefault {
		field.Default = strings.TrimSpace(defaultDomain)
	}
	return dialogs.NewFormDialog("proxy-domain", "Public domain name", description, []dialogs.Field{field})
}

func newProxyIPDialog(defaultIP string) *dialogs.FormDialog {
	field := dialogs.Field{
		ID:          "ip",
		Title:       "Public IP address",
		Description: "Used for DNS A records. Leave as-is if unchanged.",
		Required:    true,
		Default:     strings.TrimSpace(defaultIP),
		Validate: func(value string) error {
			ip := net.ParseIP(strings.TrimSpace(value))
			if ip == nil {
				return errors.New("valid IP address is required")
			}
			return nil
		},
	}
	return dialogs.NewFormDialog("proxy-ip", "Public IP address", "", []dialogs.Field{field})
}

func newProxyAuthDialog(defaultUser string, requirePassword bool) *dialogs.FormDialog {
	fields := []dialogs.Field{
		{
			ID:       "username",
			Title:    "Primary username",
			Required: true,
			Default:  strings.TrimSpace(defaultUser),
			Validate: func(value string) error {
				trimmed := strings.TrimSpace(value)
				if trimmed == "" {
					return errors.New("username is required")
				}
				if strings.ContainsAny(trimmed, " \t") {
					return errors.New("username must not contain spaces")
				}
				return nil
			},
		},
		{
			ID:       "password",
			Title:    "Password",
			Secret:   true,
			Required: requirePassword,
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return errors.New("password is required")
				}
				return nil
			},
		},
	}
	return dialogs.NewFormDialog("proxy-auth", "Primary username", "", fields)
}
