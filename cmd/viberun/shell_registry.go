// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

type HelpChild struct {
	Cmd  string
	Desc string
}

type CommandSpec struct {
	Key         string
	Display     string
	Scope       shellScope
	Aliases     []string
	Summary     string
	Description string
	Usage       string
	Options     []string
	Examples    []string
	Children    []HelpChild
	// RequiresSync gates execution until the initial host+app sync completes.
	// Keep this explicit so future commands don't accidentally bypass remote readiness.
	RequiresSync bool
}

func shellCommandSpecs() []CommandSpec {
	return []CommandSpec{
		{Key: "apps", Display: "apps", Scope: scopeGlobal, Aliases: []string{"ls"}, Summary: "list apps on the host", Description: "List apps on the host.", Usage: "apps", Examples: []string{"apps"}, RequiresSync: true},
		{Key: "app", Display: "app <name>", Scope: scopeGlobal, Summary: "enter app config mode", Description: "Enter app config mode.", Usage: "app <name>", Examples: []string{"app myapp"}, RequiresSync: true},
		{Key: "run", Display: "run <app>", Scope: scopeGlobal, Summary: "attach to the app session", Description: "Attach to the app tmux session (creates the app if it doesn't exist).", Usage: "run <app>", Examples: []string{"run myapp"}, RequiresSync: true},
		{Key: "shell", Display: "shell <app>", Scope: scopeGlobal, Summary: "open an app shell", Description: "Open a shell in the app container.", Usage: "shell <app>", Examples: []string{"shell myapp"}, RequiresSync: true},
		{Key: "config", Display: "config", Scope: scopeGlobal, Summary: "show or update local config", Description: "Show or update local configuration.", Usage: "config show | config set host <host> | config set agent <provider>", Examples: []string{"config show", "config set host root@1.2.3.4", "config set agent codex"}, RequiresSync: false, Children: []HelpChild{
			{Cmd: "config show", Desc: "show local config"},
			{Cmd: "config set host <host>", Desc: "set default host"},
			{Cmd: "config set agent <provider>", Desc: "set default agent"},
		}},
		{Key: "proxy", Display: "proxy", Scope: scopeGlobal, Summary: "configure host proxy", Description: "Configure host proxy for app URLs.", Usage: "proxy setup [host]", Examples: []string{"proxy setup"}, RequiresSync: true, Children: []HelpChild{
			{Cmd: "proxy setup [host]", Desc: "configure host proxy"},
		}},
		{Key: "users", Display: "users", Scope: scopeGlobal, Summary: "manage proxy users", Description: "Manage proxy login users.", Usage: "users list | users add --username <u> | users remove --username <u> | users set-password --username <u>", Examples: []string{"users list", "users add --username alice", "users remove --username alice", "users set-password --username alice"}, RequiresSync: true, Children: []HelpChild{
			{Cmd: "users list", Desc: "list proxy users"},
			{Cmd: "users add --username <u>", Desc: "add a user"},
			{Cmd: "users remove --username <u>", Desc: "remove a user"},
			{Cmd: "users set-password --username <u>", Desc: "set a password"},
		}},
		{Key: "help", Display: "help", Scope: scopeGlobal, Aliases: []string{"?"}, Summary: "show this help", Description: "Show help, or help for a specific command.", Usage: "help [command]", Examples: []string{"help", "help run"}, RequiresSync: false},
		{Key: "exit", Display: "exit | quit", Scope: scopeGlobal, Aliases: []string{"quit"}, Summary: "exit shell", Description: "Exit the shell.", Usage: "exit | quit", Examples: []string{"exit"}, RequiresSync: false},

		{Key: "show", Display: "show", Scope: scopeAppConfig, Summary: "show app summary", Description: "Show app summary.", Usage: "show", RequiresSync: true},
		{Key: "run", Display: "run", Scope: scopeAppConfig, Summary: "attach to the app session", Description: "Attach to the current app session (creates the app if it doesn't exist).", Usage: "run", RequiresSync: true},
		{Key: "shell", Display: "shell", Scope: scopeAppConfig, Summary: "open an app shell", Description: "Open a shell in the app container.", Usage: "shell", RequiresSync: true},
		{Key: "snapshot", Display: "snapshot", Scope: scopeAppConfig, Summary: "create a snapshot", Description: "Create a snapshot of the app volume.", Usage: "snapshot", RequiresSync: true},
		{Key: "snapshots", Display: "snapshots", Scope: scopeAppConfig, Summary: "list snapshots", Description: "List snapshots for the app.", Usage: "snapshots", RequiresSync: true},
		{Key: "restore", Display: "restore <vN|latest>", Scope: scopeAppConfig, Summary: "restore snapshot", Description: "Restore the app volume from a snapshot.", Usage: "restore <vN|latest>", Examples: []string{"restore latest", "restore v3"}, RequiresSync: true},
		{Key: "update", Display: "update", Scope: scopeAppConfig, Summary: "recreate container", Description: "Recreate the app container.", Usage: "update", RequiresSync: true},
		{Key: "delete", Display: "delete | rm", Scope: scopeAppConfig, Aliases: []string{"rm"}, Summary: "delete app", Description: "Delete the app and snapshots.", Usage: "delete | rm", RequiresSync: true},
		{Key: "url", Display: "url", Scope: scopeAppConfig, Summary: "manage app URL", Description: "Show or manage the app URL.", Usage: "url [show|open|public|private|disable|enable|set-domain <domain>|reset-domain]", Options: []string{"show", "open", "public", "private", "disable", "enable", "set-domain <domain>", "reset-domain"}, Examples: []string{"url", "url public", "url set-domain myapp.com"}, RequiresSync: true, Children: []HelpChild{
			{Cmd: "url show", Desc: "show URL info"},
			{Cmd: "url open", Desc: "open URL in browser"},
			{Cmd: "url public", Desc: "allow public access"},
			{Cmd: "url private", Desc: "require login"},
			{Cmd: "url disable", Desc: "disable the URL"},
			{Cmd: "url enable", Desc: "enable the URL"},
			{Cmd: "url set-domain <domain>", Desc: "set a custom domain"},
			{Cmd: "url reset-domain", Desc: "reset to default domain"},
		}},
		{Key: "users", Display: "users", Scope: scopeAppConfig, Summary: "manage app access", Description: "Manage app access list.", Usage: "users", RequiresSync: true},
		{Key: "help", Display: "help", Scope: scopeAppConfig, Aliases: []string{"?"}, Summary: "show this help", Description: "Show help for app commands.", Usage: "help [command]", Examples: []string{"help", "help url"}, RequiresSync: false},
		{Key: "exit", Display: "exit | back", Scope: scopeAppConfig, Aliases: []string{"back"}, Summary: "back to global", Description: "Return to the global shell.", Usage: "exit | back", RequiresSync: false},
	}
}

func commandSpecsForScope(scope shellScope) []CommandSpec {
	all := shellCommandSpecs()
	out := make([]CommandSpec, 0, len(all))
	for _, spec := range all {
		if spec.Scope == scope {
			out = append(out, spec)
		}
	}
	return out
}

func lookupCommandSpec(scope shellScope, name string) (CommandSpec, bool) {
	name = normalizeCommandName(name)
	for _, spec := range shellCommandSpecs() {
		if spec.Scope != scope {
			continue
		}
		if spec.Key == name {
			return spec, true
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return spec, true
			}
		}
	}
	return CommandSpec{}, false
}

func normalizeCommandName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
