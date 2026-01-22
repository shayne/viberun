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

	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/sshcmd"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
)

func runShellSetup(state *shellState, action setupAction) (bool, string, error) {
	hostArg := strings.TrimSpace(action.host)
	if hostArg == "" {
		if state.bootstrapped && !state.setupNeeded && strings.TrimSpace(state.host) != "" {
			rerun, err := tui.PromptSetupRerun(os.Stdin, os.Stdout, state.host)
			if err != nil {
				return false, "", err
			}
			if !rerun {
				return false, fmt.Sprintf("Okay, staying connected to %s.", state.host), nil
			}
			if _, err := runWipeFlow(state.host, false); err != nil {
				return false, "", err
			}
		}
		var err error
		hostArg, err = tui.PromptSetupHost(os.Stdin, os.Stdout, state.host)
		if err != nil {
			return false, "", err
		}
	}

	cfg := state.cfg
	cfgPath := state.cfgPath

	resolved, err := target.ResolveHost(hostArg, cfg)
	if err != nil {
		return false, "", fmt.Errorf("invalid host: %w", err)
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return false, "", fmt.Errorf("ssh is required but was not found in PATH")
	}

	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !tty {
		fmt.Fprintln(os.Stderr, "setup may require sudo; run from a terminal if you are prompted for a password")
	}
	ui := tui.NewProgress(os.Stdout, tty, "setup", resolved.Host)
	ui.Start()
	defer ui.Stop()

	env := []string{}
	for _, key := range []string{"VIBERUN_IMAGE", "VIBERUN_PROXY_IMAGE", "VIBERUN_SERVER_VERSION", "VIBERUN_SERVER_REPO"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env = append(env, key+"="+value)
		}
	}

	localBootstrap := false
	localImage := false
	updateArtifacts := true
	skipBootstrap := false
	proxyImageTag := ""
	bootstrapped, _ := checkHostBootstrapped(resolved.Host)
	if state.devMode {
		if bootstrapped && tty {
			updateArtifacts = promptYesNoDefaultYes("Update server binary and images? [Y/n]: ")
		}
		if updateArtifacts {
			localBootstrap = true
			localImage = true
		} else {
			env = append(env, "VIBERUN_SKIP_SERVER_INSTALL=1", "VIBERUN_SKIP_IMAGE_PULL=1")
		}
	} else {
		if bootstrapped && tty {
			remoteVersion, err := fetchRemoteServerVersion(resolved.Host)
			if err != nil {
				updateArtifacts = promptYesNoDefaultNo("Server version unknown. Update server now? [y/N]: ")
			} else if cmp, ok := compareSemver(version, remoteVersion); !ok {
				updateArtifacts = promptYesNoDefaultNo("Server version unknown. Update server now? [y/N]: ")
			} else if cmp > 0 {
				updateArtifacts = promptYesNoDefaultYes(fmt.Sprintf("Server is %s. Update to %s? [Y/n]: ", strings.TrimSpace(remoteVersion), strings.TrimSpace(version)))
			} else {
				updateArtifacts = false
			}
		}
		if bootstrapped && !updateArtifacts {
			skipBootstrap = true
		}
	}
	if localBootstrap {
		ui.Step("Stage server binary")
		remotePath, err := stageLocalServerBinary(resolved.Host, "")
		if err != nil {
			ui.Fail(err.Error())
			return false, "", newSilentError(fmt.Errorf("failed to stage local server binary: %w", err))
		}
		ui.Done("")
		env = append(env, "VIBERUN_SERVER_LOCAL_PATH="+remotePath)
	}
	if localImage {
		ui.Step("Build container image")
		ui.Suspend()
		if err := stageLocalImage(resolved.Host); err != nil {
			ui.Resume()
			ui.Fail(err.Error())
			return false, "", newSilentError(fmt.Errorf("failed to stage local image: %w", err))
		}
		ui.Resume()
		ui.Done("")
		ui.Step("Build proxy image")
		ui.Suspend()
		if err := stageLocalProxyImage(resolved.Host); err != nil {
			ui.Resume()
			ui.Fail(err.Error())
			return false, "", newSilentError(fmt.Errorf("failed to stage local proxy image: %w", err))
		}
		ui.Resume()
		ui.Done("")
		env = append(env, "VIBERUN_SKIP_IMAGE_PULL=1")
		if state.devMode {
			proxyImageTag = devProxyImageTag()
		}
	}

	command := bootstrapCommand(bootstrapScript())
	remoteArgs := []string{"bash", "-lc", shellQuote(command)}
	if len(env) > 0 {
		remoteArgs = append([]string{"env"}, append(env, remoteArgs...)...)
	}
	sshArgs := sshcmd.BuildArgs(resolved.Host, remoteArgs, tty)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if skipBootstrap {
		ui.Step("Run bootstrap")
		ui.Done("skipped")
	} else {
		ui.Step("Run bootstrap")
		ui.Suspend()
		if err := cmd.Run(); err != nil {
			ui.Resume()
			ui.Fail(err.Error())
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return false, "", newSilentError(exitErr)
			}
			return false, "", fmt.Errorf("failed to start ssh: %w", err)
		}
		ui.Resume()
		ui.Done("")
	}

	if strings.TrimSpace(hostArg) != "" && strings.TrimSpace(cfg.DefaultHost) != hostArg {
		cfg.DefaultHost = hostArg
		if err := config.Save(cfgPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Setup complete, but failed to save default host: %v\n", err)
			fmt.Fprintf(os.Stderr, "Run `viberun config --host %s` to set it manually.\n", hostArg)
		} else {
			state.cfg = cfg
			state.host = hostArg
			state.hostPrompt = false
			fmt.Fprintf(os.Stdout, "default host set to %s\n", hostArg)
		}
	}

	if tty {
		if err := runProxySetupFlow(resolved.Host, proxySetupOptions{updateArtifacts: updateArtifacts, showSkipHint: true, proxyImage: proxyImageTag}); err != nil {
			fmt.Fprintf(os.Stderr, "proxy setup failed: %v\n", err)
		}
	}
	fmt.Fprintln(os.Stdout, "Setup complete.")
	return true, "", nil
}
