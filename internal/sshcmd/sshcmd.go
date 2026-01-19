// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshcmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RemoteArgs builds the remote command executed on the server host.
func RemoteArgs(app string, agentProvider string, actionArgs []string, extraEnv map[string]string) []string {
	if strings.TrimSpace(agentProvider) == "" {
		agentProvider = "codex"
	}
	remote := []string{"viberun-server", "--agent", agentProvider, app}
	remote = append(remote, actionArgs...)
	entries := map[string]string{}
	if value := strings.TrimSpace(os.Getenv("VIBERUN_AGENT_CHECK")); value != "" {
		entries["VIBERUN_AGENT_CHECK"] = value
	}
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		entries[key] = value
	}
	if len(entries) == 0 {
		return remote
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	prefix := []string{"env"}
	for _, key := range keys {
		prefix = append(prefix, key+"="+entries[key])
	}
	return append(prefix, remote...)
}

// WithSudo wraps the remote viberun-server command in sudo when the host user is non-root.
func WithSudo(host string, remoteArgs []string) []string {
	if !needsSudo(host) {
		return remoteArgs
	}
	if len(remoteArgs) == 0 {
		return remoteArgs
	}
	cmdIdx := 0
	if remoteArgs[0] == "env" {
		cmdIdx = 1
		for cmdIdx < len(remoteArgs) && strings.Contains(remoteArgs[cmdIdx], "=") {
			cmdIdx++
		}
	}
	if cmdIdx >= len(remoteArgs) {
		return remoteArgs
	}
	cmd := remoteArgs[cmdIdx]
	if filepath.Base(cmd) != "viberun-server" {
		return remoteArgs
	}
	sudo := []string{"sudo", "-n", "/usr/local/bin/viberun-server"}
	out := make([]string, 0, len(remoteArgs)+2)
	out = append(out, remoteArgs[:cmdIdx]...)
	out = append(out, sudo...)
	out = append(out, remoteArgs[cmdIdx+1:]...)
	return out
}

func needsSudo(host string) bool {
	user := hostUser(host)
	return user != "" && user != "root"
}

func hostUser(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if parts := strings.SplitN(host, "://", 2); len(parts) == 2 {
		if parsed, err := url.Parse(host); err == nil && parsed.User != nil {
			return parsed.User.Username()
		}
		return ""
	}
	if at := strings.LastIndex(host, "@"); at >= 0 {
		return strings.TrimSpace(host[:at])
	}
	return ""
}

// BuildArgs builds the ssh argument list for a target host and remote command.
func BuildArgs(host string, remoteArgs []string, tty bool) []string {
	return BuildArgsWithForwards(host, remoteArgs, tty, nil, nil)
}

type LocalForward struct {
	LocalPort  int
	RemoteHost string
	RemotePort int
}

type RemoteSocketForward struct {
	RemotePath string
	LocalHost  string
	LocalPort  int
}

// BuildArgsWithLocalForward builds the ssh argument list for a target host and optional local forward.
func BuildArgsWithLocalForward(host string, remoteArgs []string, tty bool, forward *LocalForward) []string {
	return BuildArgsWithForwards(host, remoteArgs, tty, forward, nil)
}

// BuildPortForwardArgs builds ssh args for a background local port forward only.
func BuildPortForwardArgs(host string, forward *LocalForward) []string {
	args := []string{"-T", "-N", "-o", "ExitOnForwardFailure=yes"}
	if forward != nil {
		remoteHost := strings.TrimSpace(forward.RemoteHost)
		if remoteHost == "" {
			remoteHost = "localhost"
		}
		args = append(args, "-L", fmt.Sprintf("%d:%s:%d", forward.LocalPort, remoteHost, forward.RemotePort))
	}
	args = append(args, host)
	return args
}

// BuildArgsWithForwards builds the ssh argument list for a target host with optional forwards.
func BuildArgsWithForwards(host string, remoteArgs []string, tty bool, forward *LocalForward, remoteSocket *RemoteSocketForward) []string {
	args := []string{}
	if tty {
		args = append(args, "-tt")
	} else {
		args = append(args, "-T")
	}
	if forward != nil {
		remoteHost := strings.TrimSpace(forward.RemoteHost)
		if remoteHost == "" {
			remoteHost = "localhost"
		}
		args = append(args, "-L", fmt.Sprintf("%d:%s:%d", forward.LocalPort, remoteHost, forward.RemotePort))
	}
	if remoteSocket != nil {
		localHost := strings.TrimSpace(remoteSocket.LocalHost)
		if localHost == "" {
			localHost = "localhost"
		}
		args = append(args, "-o", "ExitOnForwardFailure=yes", "-o", "StreamLocalBindUnlink=yes", "-o", "StreamLocalBindMask=0111")
		args = append(args, "-R", fmt.Sprintf("%s:%s:%d", remoteSocket.RemotePath, localHost, remoteSocket.LocalPort))
	}
	args = append(args, host)
	return append(args, remoteArgs...)
}
