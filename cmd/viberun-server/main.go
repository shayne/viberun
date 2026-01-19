// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/shayne/viberun/internal/agents"
	"github.com/shayne/viberun/internal/server"
	"github.com/shayne/viberun/internal/tui"
	"github.com/shayne/yargs"
)

const defaultImage = "viberun:latest"

type serverFlags struct {
	Agent string `flag:"agent" help:"agent provider to run (codex, claude, gemini, ampcode, opencode, or npx:<pkg>/uvx:<pkg>)"`
}

type SnapshotInfo struct {
	Tag          string
	CreatedAt    time.Time
	CreatedAtRaw string
}

type restoreQueue struct {
	mu         sync.Mutex
	inProgress bool
	ref        string
	ch         chan struct{}
}

var errRestoreInProgress = errors.New("restore already in progress")

func newRestoreQueue() *restoreQueue {
	return &restoreQueue{ch: make(chan struct{}, 1)}
}

func (q *restoreQueue) Enqueue(ref string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.inProgress {
		return errRestoreInProgress
	}
	q.inProgress = true
	q.ref = ref
	select {
	case q.ch <- struct{}{}:
	default:
	}
	return nil
}

func (q *restoreQueue) Next() (string, bool) {
	select {
	case <-q.ch:
		q.mu.Lock()
		ref := q.ref
		q.mu.Unlock()
		return ref, true
	default:
		return "", false
	}
}

func (q *restoreQueue) Current() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.ref
}

func (q *restoreQueue) Pending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.inProgress
}

func (q *restoreQueue) Finish() {
	q.mu.Lock()
	q.inProgress = false
	q.ref = ""
	q.mu.Unlock()
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || hasHelpFlag(args) {
		fmt.Fprintln(os.Stderr, "Usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|update|shell|port|delete|exists]")
		os.Exit(2)
	}
	result, err := yargs.ParseFlags[serverFlags](args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	if len(result.Args) < 1 || len(result.Args) > 3 {
		fmt.Fprintln(os.Stderr, "Usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|update|shell|port|delete|exists]")
		os.Exit(2)
	}
	args = result.Args
	app := strings.TrimSpace(args[0])
	if app == "" {
		fmt.Fprintln(os.Stderr, "app name is required")
		os.Exit(2)
	}

	action, actionArgs, err := parseAction(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	agentProvider := strings.TrimSpace(result.Flags.Agent)
	agentSpec, err := agents.Resolve(agentProvider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid agent provider: %v\n", err)
		os.Exit(2)
	}
	agentArgs := agentSpec.Command
	agentLabel := agentSpec.Label
	sessionName := "viberun-agent"
	if action == "shell" {
		agentArgs = []string{"/bin/bash"}
		agentLabel = "shell"
		sessionName = "viberun-shell"
	}
	agentArgs = tmuxSessionArgs(sessionName, agentLabel, agentArgs)

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "viberun-server must run as root; run via sudo or re-run bootstrap")
		os.Exit(1)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Fprintln(os.Stderr, "docker is required but was not found in PATH")
		os.Exit(1)
	}
	if err := ensureRootfulDocker(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	state, statePath, err := server.LoadState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load server state: %v\n", err)
		os.Exit(1)
	}
	stateDirty := false
	if synced, err := syncPortsFromContainers(&state); err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync port mappings: %v\n", err)
		os.Exit(1)
	} else if synced {
		stateDirty = true
	}

	containerName := fmt.Sprintf("viberun-%s", app)
	exists, err := containerExists(containerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to inspect container: %v\n", err)
		os.Exit(1)
	}

	if action == "exists" {
		fmt.Fprintln(os.Stdout, exists)
		return
	}

	if action == "snapshot" {
		if !exists {
			fmt.Fprintln(os.Stderr, "cannot snapshot: app container does not exist")
			os.Exit(1)
		}
		if _, ok, err := ensureHomeVolume(app, false); err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("app volume does not exist")
			}
			fmt.Fprintf(os.Stderr, "failed to access app volume: %v\n", err)
			os.Exit(1)
		}
		ref, err := createSnapshot(containerName, app)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "Snapshot created: %s\n", ref)
		return
	}
	if action == "snapshots" {
		lines, err := listSnapshotLines(app)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list snapshots: %v\n", err)
			os.Exit(1)
		}
		if len(lines) == 0 {
			fmt.Fprintf(os.Stdout, "No snapshots found for %s\n", app)
			return
		}
		fmt.Fprintf(os.Stdout, "Snapshots for %s:\n", app)
		for _, line := range lines {
			fmt.Fprintf(os.Stdout, "  %s\n", line)
		}
		return
	}
	if action == "delete" {
		deletedState, err := deleteApp(containerName, app, &state, exists)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to delete app: %v\n", err)
			os.Exit(1)
		}
		if deletedState {
			stateDirty = true
		}
		if stateDirty {
			if err := server.SaveState(statePath, state); err != nil {
				fmt.Fprintf(os.Stderr, "failed to save server state: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stdout, "Deleted app %s\n", app)
		return
	}

	port, portDirty, err := resolvePort(&state, app, containerName, exists)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if portDirty {
		stateDirty = true
	}

	if action == "port" {
		if stateDirty {
			if err := server.SaveState(statePath, state); err != nil {
				fmt.Fprintf(os.Stderr, "failed to save server state: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintln(os.Stdout, port)
		return
	}

	if action == "update" {
		if !exists {
			fmt.Fprintln(os.Stderr, "cannot update: app container does not exist")
			os.Exit(1)
		}
		if _, ok, err := ensureHomeVolume(app, false); err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("app volume does not exist")
			}
			fmt.Fprintf(os.Stderr, "failed to access app volume: %v\n", err)
			os.Exit(1)
		}
		ui := newAppProgress(app)
		ui.Start()
		ui.Step("Pull image")
		if err := runDockerCommandOutput("pull", defaultImage); err != nil {
			ui.Fail("failed")
			fmt.Fprintf(os.Stderr, "failed to pull image: %v\n", err)
			os.Exit(1)
		}
		ui.Done("")
		ui.Step("Recreate container")
		if err := runDockerCommandOutput("rm", "-f", containerName); err != nil {
			ui.Fail("failed")
			fmt.Fprintf(os.Stderr, "failed to remove container: %v\n", err)
			os.Exit(1)
		}
		if err := dockerRun(containerName, app, port); err != nil {
			ui.Fail("failed")
			fmt.Fprintf(os.Stderr, "failed to create container: %v\n", err)
			os.Exit(1)
		}
		ui.Done("")
		ui.Stop()
		_ = clearUpdateStatus(app)
		if stateDirty {
			if err := server.SaveState(statePath, state); err != nil {
				fmt.Fprintf(os.Stderr, "failed to save server state: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stdout, "Updated app %s\n", app)
		return
	}

	if action == "restore" {
		ref, err := resolveSnapshotRef(app, actionArgs[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve snapshot: %v\n", err)
			os.Exit(1)
		}
		if err := restoreSnapshot(containerName, app, port, ref); err != nil {
			fmt.Fprintf(os.Stderr, "failed to restore snapshot: %v\n", err)
			os.Exit(1)
		}
		if stateDirty {
			if err := server.SaveState(statePath, state); err != nil {
				fmt.Fprintf(os.Stderr, "failed to save server state: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stdout, "Restored app %s from %s\n", app, ref)
		return
	}

	var ui *tui.Progress
	if !exists {
		if !autoCreateEnabled() {
			if !promptCreate(app) {
				fmt.Fprintln(os.Stderr, "aborted")
				os.Exit(1)
			}
		}

		ui = newAppProgress(app)
		ui.Start()
		ui.Step("Prepare volume")
		if _, _, err := ensureHomeVolume(app, true); err != nil {
			ui.Fail("failed")
			fmt.Fprintf(os.Stderr, "failed to prepare app volume: %v\n", err)
			os.Exit(1)
		}
		ui.Done("")
		ui.Step("Start container")
		if err := dockerRun(containerName, app, port); err != nil {
			ui.Fail("failed")
			fmt.Fprintf(os.Stderr, "failed to create container: %v\n", err)
			os.Exit(1)
		}
		ui.Done("")
	} else {
		running, err := containerRunning(containerName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to check container state: %v\n", err)
			os.Exit(1)
		}
		if _, ok, err := ensureHomeVolume(app, false); err != nil || !ok {
			if err == nil {
				err = fmt.Errorf("app volume does not exist")
			}
			fmt.Fprintf(os.Stderr, "failed to prepare app volume: %v\n", err)
			os.Exit(1)
		}
		if !running {
			ui = newAppProgress(app)
			ui.Start()
			ui.Step("Start container")
			if err := dockerStart(containerName); err != nil {
				ui.Fail("failed")
				fmt.Fprintf(os.Stderr, "failed to start container: %v\n", err)
				os.Exit(1)
			}
			ui.Done("")
		}
	}
	if ui != nil {
		ui.Stop()
	}

	if stateDirty {
		if err := server.SaveState(statePath, state); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save server state: %v\n", err)
			os.Exit(1)
		}
	}

	running, err := containerRunning(containerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check container state: %v\n", err)
		os.Exit(1)
	}
	if !running {
		explainStoppedContainer(containerName)
		os.Exit(1)
	}

	if strings.TrimSpace(os.Getenv("VIBERUN_XDG_OPEN_SOCKET")) != "" {
		_, _ = xdgOpenSocketPath()
	}

	authBundle, err := loadAuthBundleFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load auth bundle: %v\n", err)
		os.Exit(1)
	}
	var bundleEnv map[string]string
	if !exists && authBundle != nil {
		if err := applyAuthBundle(containerName, authBundle); err != nil {
			fmt.Fprintf(os.Stderr, "failed to apply auth bundle: %v\n", err)
			os.Exit(1)
		}
		if len(authBundle.Env) > 0 {
			bundleEnv = authBundle.Env
		}
	}

	if action == "" || action == "shell" {
		restoreQueue := newRestoreQueue()
		hostRPC, extraEnv, err := startHostRPC(app, containerName, port, createSnapshot, listSnapshotLines, func(_ string, _ string, _ int, snapshotRef string) error {
			if err := ensureSnapshotExists(app, snapshotRef); err != nil {
				return err
			}
			return restoreQueue.Enqueue(snapshotRef)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start host rpc: %v\n", err)
			os.Exit(1)
		}
		stopUpdates := startUpdateWatcher(app, containerName, time.Hour)
		for key, value := range bundleEnv {
			extraEnv[key] = value
		}
		if strings.TrimSpace(agentLabel) != "" {
			extraEnv["VIBERUN_AGENT"] = agentLabel
		}
		if action == "" {
			if err := checkCustomAgent(app, containerName, agentSpec, extraEnv); err != nil {
				if hostRPC != nil {
					_ = hostRPC.Close()
				}
				if stopUpdates != nil {
					stopUpdates()
				}
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		if err := runInteractiveSession(containerName, app, port, agentArgs, extraEnv, restoreQueue); err != nil {
			if stopUpdates != nil {
				stopUpdates()
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "session ended: %v\n", err)
			os.Exit(1)
		}
		if stopUpdates != nil {
			stopUpdates()
		}
		if hostRPC != nil {
			_ = hostRPC.Close()
		}
		return
	}
}

func parseAction(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, nil
	}
	if len(args) == 1 && args[0] == "snapshot" {
		return "snapshot", nil, nil
	}
	if len(args) == 1 && args[0] == "snapshots" {
		return "snapshots", nil, nil
	}
	if len(args) == 1 && args[0] == "update" {
		return "update", nil, nil
	}
	if len(args) == 1 && args[0] == "shell" {
		return "shell", nil, nil
	}
	if len(args) == 1 && args[0] == "port" {
		return "port", nil, nil
	}
	if len(args) == 1 && args[0] == "exists" {
		return "exists", nil, nil
	}
	if len(args) == 1 && args[0] == "delete" {
		return "delete", nil, nil
	}
	if len(args) == 2 && args[0] == "restore" && strings.TrimSpace(args[1]) != "" {
		return "restore", []string{strings.TrimSpace(args[1])}, nil
	}
	return "", nil, fmt.Errorf("usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|update|shell|port|delete|exists]")
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "-h", "--help", "--help-llm", "help":
			return true
		}
	}
	return false
}

func resolvePort(state *server.State, app string, containerName string, exists bool) (int, bool, error) {
	port, ok := state.PortForApp(app)
	stateDirty := false
	if exists && !ok {
		discovered, found, err := containerPort(containerName)
		if err != nil {
			return 0, false, fmt.Errorf("failed to read container port: %v", err)
		}
		if found {
			state.SetPort(app, discovered)
			port = discovered
			stateDirty = true
		} else {
			return 0, false, fmt.Errorf("existing container has no host port mapping for 8080; recreate or restore the app")
		}
	}
	if port == 0 {
		port = state.AssignPort(app)
		stateDirty = true
	}
	return port, stateDirty, nil
}

func promptCreate(app string) bool {
	return promptCreateWithReader(app, os.Stdin, os.Stdout)
}

func promptCreateWithReader(app string, in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	fmt.Fprintf(out, "App %s does not exist. Create? [Y/n]: ", app)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" || input == "y" || input == "yes" {
		return true
	}
	return false
}

func containerExists(name string) (bool, error) {
	cmd := exec.Command("docker", "inspect", name)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func containerRunning(name string) (bool, error) {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name).Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func containerPort(name string) (int, bool, error) {
	out, err := exec.Command("docker", "port", name, "8080/tcp").Output()
	if err != nil {
		return 0, false, err
	}

	port, found := parsePortMapping(string(out))
	return port, found, nil
}

func parsePortMapping(output string) (int, bool) {
	re := regexp.MustCompile(`:(\d+)$`)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		match := re.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		port, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		return port, true
	}
	return 0, false
}

func syncPortsFromContainers(state *server.State) (bool, error) {
	containers, err := listContainers()
	if err != nil {
		return false, err
	}

	updated := false
	for _, name := range containers {
		if !strings.HasPrefix(name, "viberun-") {
			continue
		}
		app := strings.TrimPrefix(name, "viberun-")
		if app == "" {
			continue
		}
		if _, ok := state.PortForApp(app); ok {
			continue
		}
		port, found, err := containerPort(name)
		if err != nil {
			continue
		}
		if !found {
			continue
		}
		state.SetPort(app, port)
		updated = true
	}

	return updated, nil
}

func listContainers() ([]string, error) {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var names []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func dockerRun(name string, app string, port int) error {
	if err := ensureHostRPCDir(app); err != nil {
		return err
	}
	args := dockerRunArgs(name, app, port, defaultImage)
	return runDockerCommandOutput(args...)
}

func dockerStart(name string) error {
	return runDockerCommandOutput("start", name)
}

func dockerExec(name string, agentArgs []string, extraEnv map[string]string) error {
	if len(agentArgs) == 0 {
		agentArgs = []string{"/bin/bash"}
	}
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	env := map[string]string{}
	if tty {
		env["TERM"] = normalizeTermValue(os.Getenv("TERM"))
		if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
			env["COLORTERM"] = colorTerm
		}
	}
	if agentCheck := strings.TrimSpace(os.Getenv("VIBERUN_AGENT_CHECK")); agentCheck != "" {
		env["VIBERUN_AGENT_CHECK"] = agentCheck
	}
	if forwardAgentEnabled() {
		if socketPath, ok := sshAuthSocketPath(); ok {
			socketDir := filepath.Dir(socketPath)
			mounted, err := containerHasMountSource(name, socketDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to inspect agent forwarding mount: %v\n", err)
			} else if !mounted {
				fmt.Fprintln(os.Stderr, "SSH agent forwarding isn't available in this container. Run `viberun --forward-agent <app> update` to enable it.")
			} else {
				env["SSH_AUTH_SOCK"] = socketPath
				_ = runDockerCommandOutput("exec", name, "tmux", "set-environment", "-g", "SSH_AUTH_SOCK", socketPath)
			}
		} else {
			_ = runDockerCommandOutput("exec", name, "tmux", "set-environment", "-g", "-u", "SSH_AUTH_SOCK")
		}
	}
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		env[key] = value
	}
	args := dockerExecArgs(name, agentArgs, tty, env)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func normalizeTermValue(termValue string) string {
	value := strings.TrimSpace(termValue)
	if value == "" {
		return "xterm-256color"
	}
	return value
}

func newAppProgress(app string) *tui.Progress {
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	return tui.NewProgress(os.Stdout, tty, app, "")
}

func dockerExecArgs(name string, agentArgs []string, tty bool, env map[string]string) []string {
	args := []string{"exec", "-i"}
	if tty {
		args = append(args, "-t")
	}
	user := strings.TrimSpace(os.Getenv("VIBERUN_CONTAINER_USER"))
	if user == "" {
		user = "viberun"
	}
	args = append(args, "--user", user)
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for key := range env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "-e", fmt.Sprintf("%s=%s", key, env[key]))
		}
	}
	args = append(args, name)
	return append(args, agentArgs...)
}

func explainStoppedContainer(name string) {
	if mismatch, imageArch, hostArch, err := containerArchMismatch(name); err == nil && mismatch {
		fmt.Fprintf(os.Stderr, "container image architecture mismatch (image=%s, host=%s)\n", imageArch, hostArch)
		fmt.Fprintf(os.Stderr, "run `viberun %s --delete -y` to recreate with the correct image\n", name)
		return
	}
	if tail, err := containerLogsTail(name, 5); err == nil && strings.TrimSpace(tail) != "" {
		fmt.Fprintf(os.Stderr, "container stopped; last logs:\n%s\n", tail)
	}
	fmt.Fprintf(os.Stderr, "container %s is not running\n", name)
}

func containerArchMismatch(name string) (bool, string, string, error) {
	imageID, err := containerImageID(name)
	if err != nil {
		return false, "", "", err
	}
	imageArch, err := imageArchitecture(imageID)
	if err != nil {
		return false, "", "", err
	}
	hostArch, err := hostArchitecture()
	if err != nil {
		return false, "", "", err
	}
	return imageArch != hostArch, imageArch, hostArch, nil
}

func containerImageID(name string) (string, error) {
	out, err := exec.Command("docker", "inspect", "-f", "{{.Image}}", name).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func imageArchitecture(image string) (string, error) {
	out, err := exec.Command("docker", "image", "inspect", "-f", "{{.Architecture}}", image).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func hostArchitecture() (string, error) {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", err
	}
	value := strings.ToLower(strings.TrimSpace(string(out)))
	switch value {
	case "x86_64", "amd64":
		return "amd64", nil
	case "arm64", "aarch64":
		return "arm64", nil
	default:
		return value, nil
	}
}

func containerLogsTail(name string, lines int) (string, error) {
	if lines < 1 {
		lines = 1
	}
	out, err := exec.Command("docker", "logs", "--tail", strconv.Itoa(lines), name).CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func deleteApp(containerName string, app string, state *server.State, exists bool) (bool, error) {
	removed := false
	if exists {
		if err := runDockerCommandOutput("rm", "-f", containerName); err != nil {
			return false, err
		}
	}
	if err := deleteHomeVolume(app); err != nil {
		return false, err
	}
	if err := deleteHostRPCDir(app); err != nil {
		return false, err
	}
	if state != nil {
		removed = state.RemoveApp(app)
	}
	return removed, nil
}

func createSnapshot(containerName string, app string) (string, error) {
	cfg, ok, err := ensureHomeVolume(app, false)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("app volume does not exist")
	}
	infos, err := listSnapshotInfos(app)
	if err != nil {
		return "", err
	}
	tag := nextSnapshotTagFromInfos(infos)
	if err := snapshotContainer(containerName, cfg, tag); err != nil {
		return "", err
	}
	return tag, nil
}

func resolveSnapshotRef(app string, name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	if normalized == "latest" {
		return latestSnapshotRef(app)
	}
	if tag, ok := snapshotTag(normalized); ok {
		return tag, nil
	}
	return "", fmt.Errorf("invalid snapshot name: %s", name)
}

func nextSnapshotTagFromInfos(infos []SnapshotInfo) string {
	maxVersion := 0
	for _, info := range infos {
		if version, ok := parseSnapshotTag(info.Tag); ok && version > maxVersion {
			maxVersion = version
		}
	}
	return fmt.Sprintf("v%d", maxVersion+1)
}

func listSnapshotInfos(app string) ([]SnapshotInfo, error) {
	cfg, ok, err := ensureHomeVolume(app, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	entries, err := os.ReadDir(cfg.SnapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var infos []SnapshotInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tag := strings.TrimSpace(entry.Name())
		if tag == "" {
			continue
		}
		if _, ok := snapshotTag(tag); !ok {
			continue
		}
		info := SnapshotInfo{Tag: tag}
		if stat, err := entry.Info(); err == nil {
			info.CreatedAt = stat.ModTime()
			if !info.CreatedAt.IsZero() {
				info.CreatedAtRaw = info.CreatedAt.Format(time.RFC3339)
			}
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func listSnapshotLines(app string) ([]string, error) {
	infos, err := listSnapshotInfos(app)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, nil
	}
	sortSnapshotInfos(infos)
	lines := make([]string, 0, len(infos))
	for _, info := range infos {
		lines = append(lines, formatSnapshotLine(info))
	}
	return lines, nil
}

func sortSnapshotInfos(infos []SnapshotInfo) {
	sort.Slice(infos, func(i, j int) bool {
		left, right := infos[i], infos[j]
		leftVersion, leftOk := parseSnapshotTag(left.Tag)
		rightVersion, rightOk := parseSnapshotTag(right.Tag)
		if leftOk && rightOk {
			if leftVersion != rightVersion {
				return leftVersion < rightVersion
			}
		} else if leftOk != rightOk {
			return leftOk
		}
		if !left.CreatedAt.IsZero() && !right.CreatedAt.IsZero() {
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
		}
		return left.Tag < right.Tag
	})
}

func formatSnapshotLine(info SnapshotInfo) string {
	if info.CreatedAtRaw != "" {
		return fmt.Sprintf("%s %s", info.Tag, info.CreatedAtRaw)
	}
	if !info.CreatedAt.IsZero() {
		return fmt.Sprintf("%s %s", info.Tag, info.CreatedAt.Format(time.RFC3339))
	}
	return info.Tag
}

func latestSnapshotRefFromInfos(app string, infos []SnapshotInfo) (string, error) {
	if len(infos) == 0 {
		return "", fmt.Errorf("no snapshots found for %s", app)
	}
	maxVersion := 0
	tag := ""
	for _, info := range infos {
		if version, ok := parseSnapshotTag(info.Tag); ok && version > maxVersion {
			maxVersion = version
			tag = info.Tag
		}
	}
	if maxVersion > 0 && tag != "" {
		return tag, nil
	}
	best := infos[0]
	for _, info := range infos[1:] {
		switch {
		case best.CreatedAt.IsZero() && !info.CreatedAt.IsZero():
			best = info
		case !best.CreatedAt.IsZero() && !info.CreatedAt.IsZero() && info.CreatedAt.After(best.CreatedAt):
			best = info
		case best.CreatedAt.IsZero() && info.CreatedAt.IsZero() && info.Tag > best.Tag:
			best = info
		}
	}
	return best.Tag, nil
}

func latestSnapshotRef(app string) (string, error) {
	infos, err := listSnapshotInfos(app)
	if err != nil {
		return "", err
	}
	return latestSnapshotRefFromInfos(app, infos)
}

func ensureSnapshotExists(app string, snapshotRef string) error {
	exists, err := snapshotExists(app, snapshotRef)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("snapshot not found: %s", snapshotRef)
	}
	return nil
}

func snapshotExists(app string, snapshotRef string) (bool, error) {
	if strings.TrimSpace(snapshotRef) == "" {
		return false, fmt.Errorf("snapshot ref required")
	}
	cfg, ok, err := ensureHomeVolume(app, false)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if _, err := os.Stat(snapshotPathForTag(cfg, snapshotRef)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func restoreSnapshot(containerName string, app string, port int, snapshotRef string) error {
	cfg, ok, err := ensureHomeVolume(app, false)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("app volume does not exist")
	}
	if err := ensureSnapshotExists(app, snapshotRef); err != nil {
		return err
	}
	exists, err := containerExists(containerName)
	if err != nil {
		return err
	}
	if exists {
		_ = runDockerCommandOutput("stop", containerName)
	}
	if err := restoreHomeVolume(cfg, snapshotRef); err != nil {
		return err
	}
	if exists {
		if err := dockerStart(containerName); err != nil {
			return err
		}
	} else {
		if err := dockerRun(containerName, app, port); err != nil {
			return err
		}
	}
	return nil
}

func waitForContainerRunning(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		running, err := containerRunning(name)
		if err == nil && running {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("container did not start: %v", err)
			}
			return fmt.Errorf("container did not start in time")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func writeRestoreBanner(out io.Writer, snapshotRef string) {
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Restoring snapshot %s...\n", snapshotRef)
	fmt.Fprintln(out, "Please wait; reconnecting to tmux.")
}

func writeRestoreFailure(out io.Writer, app string, err error) {
	fmt.Fprintf(out, "Restore failed: %v\n", err)
	fmt.Fprintf(out, "Reconnect with: viberun %s\n", app)
	fmt.Fprintf(out, "Or retry restore with: viberun %s restore latest\n", app)
}

func performRestore(containerName string, app string, port int, snapshotRef string, execDone <-chan error) error {
	writeRestoreBanner(os.Stdout, snapshotRef)
	if execDone != nil {
		_ = exec.Command("docker", "stop", containerName).Run()
		<-execDone
	}
	return restoreSnapshot(containerName, app, port, snapshotRef)
}

func runInteractiveSession(containerName string, app string, port int, agentArgs []string, env map[string]string, restores *restoreQueue) error {
	for {
		execDone := make(chan error, 1)
		go func() {
			execDone <- dockerExec(containerName, agentArgs, env)
		}()

		select {
		case <-restores.ch:
			snapshotRef := restores.Current()
			restoreErr := performRestore(containerName, app, port, snapshotRef, execDone)
			restores.Finish()
			if restoreErr != nil {
				writeRestoreFailure(os.Stderr, app, restoreErr)
				return restoreErr
			}
			if err := waitForContainerRunning(containerName, 30*time.Second); err != nil {
				writeRestoreFailure(os.Stderr, app, err)
				return err
			}
			fmt.Fprintln(os.Stdout, "Reconnected.")
			continue
		case err := <-execDone:
			if restores.Pending() {
				select {
				case <-restores.ch:
				default:
				}
				snapshotRef := restores.Current()
				restoreErr := performRestore(containerName, app, port, snapshotRef, nil)
				restores.Finish()
				if restoreErr != nil {
					writeRestoreFailure(os.Stderr, app, restoreErr)
					return restoreErr
				}
				if err := waitForContainerRunning(containerName, 30*time.Second); err != nil {
					writeRestoreFailure(os.Stderr, app, err)
					return err
				}
				fmt.Fprintln(os.Stdout, "Reconnected.")
				continue
			}
			if err == nil {
				return nil
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr
			}
			return err
		}
	}
}

func dockerRunArgs(name string, app string, port int, image string) []string {
	hostRPC := hostRPCConfigForApp(app)
	homeCfg := homeVolumeConfigForApp(app)
	args := []string{
		"run",
		"-d",
		"--name",
		name,
		"-p",
		fmt.Sprintf("%d:8080", port),
		"-e",
		"VIBERUN_APP_PORT=8080",
		"-e",
		fmt.Sprintf("VIBERUN_HOST_PORT=%d", port),
		"-e",
		fmt.Sprintf("VIBERUN_APP=%s", app),
		"-e",
		fmt.Sprintf("VIBERUN_CONTAINER=%s", name),
		"-e",
		fmt.Sprintf("VIBERUN_PORT=%d", port),
	}
	args = append(args,
		"-v",
		fmt.Sprintf("%s:%s", homeCfg.MountDir, "/home/viberun"),
	)
	args = append(args,
		"-v",
		fmt.Sprintf("%s:%s", hostRPC.HostDir, hostRPC.ContainerDir),
		"-e",
		fmt.Sprintf("VIBERUN_HOST_RPC_SOCKET=%s", hostRPC.ContainerSocket),
		"-e",
		fmt.Sprintf("VIBERUN_HOST_RPC_TOKEN_FILE=%s", hostRPC.ContainerTokenFile),
	)
	if socketPath, ok := xdgOpenSocketPath(); ok {
		socketDir := filepath.Dir(socketPath)
		args = append(args,
			"-v",
			fmt.Sprintf("%s:%s", socketDir, socketDir),
			"-e",
			fmt.Sprintf("VIBERUN_XDG_OPEN_SOCKET=%s", socketPath),
		)
	}
	if socketPath, ok := sshAuthSocketPath(); ok {
		socketDir := filepath.Dir(socketPath)
		args = append(args,
			"-v",
			fmt.Sprintf("%s:%s", socketDir, socketDir),
			"-e",
			fmt.Sprintf("SSH_AUTH_SOCK=%s", socketPath),
		)
	}
	args = append(args, image, "/usr/bin/s6-svscan", "/home/viberun/.local/services")
	return args
}

func tmuxSessionArgs(session string, windowName string, command []string) []string {
	if strings.TrimSpace(session) == "" {
		session = "viberun-session"
	}
	if len(command) == 0 {
		command = []string{"/bin/bash"}
	}
	args := []string{"tmux", "new-session", "-A", "-s", session}
	if strings.TrimSpace(windowName) != "" {
		args = append(args, "-n", windowName)
	}
	return append(args, command...)
}

func xdgOpenSocketPath() (string, bool) {
	socket := strings.TrimSpace(os.Getenv("VIBERUN_XDG_OPEN_SOCKET"))
	if socket == "" {
		return "", false
	}
	if waitForSocket(socket, 10, 100*time.Millisecond) {
		if err := os.Chmod(socket, 0o666); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to chmod VIBERUN_XDG_OPEN_SOCKET at %s: %v\n", socket, err)
		}
		return socket, true
	}
	fmt.Fprintf(os.Stderr, "warning: VIBERUN_XDG_OPEN_SOCKET is set but socket not found at %s\n", socket)
	return "", false
}

func sshAuthSocketPath() (string, bool) {
	if !forwardAgentEnabled() {
		return "", false
	}
	socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if socket == "" {
		return "", false
	}
	if isSocket(socket) {
		ensureAgentSocketAccess(socket)
		return socket, true
	}
	fmt.Fprintf(os.Stderr, "warning: VIBERUN_FORWARD_AGENT is set but SSH_AUTH_SOCK is not a socket at %s\n", socket)
	return "", false
}

func waitForSocket(path string, attempts int, delay time.Duration) bool {
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if isSocket(path) {
			return true
		}
		time.Sleep(delay)
	}
	return false
}

func autoCreateEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("VIBERUN_AUTO_CREATE")))
	switch value {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func forwardAgentEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("VIBERUN_FORWARD_AGENT")))
	switch value {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

func isSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

func ensureAgentSocketAccess(socketPath string) {
	dir := filepath.Dir(socketPath)
	if err := chmodPath(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to chmod SSH agent dir %s: %v\n", dir, err)
	}
	if err := chmodPath(socketPath, 0o666); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to chmod SSH agent socket %s: %v\n", socketPath, err)
	}
}

func chmodPath(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err == nil {
		return nil
	}
	return runHostCommand("chmod", fmt.Sprintf("%#o", mode), path).Run()
}

func ensureRootfulDocker() error {
	if host := strings.TrimSpace(os.Getenv("DOCKER_HOST")); host != "" {
		if strings.HasPrefix(host, "unix://") {
			socket := strings.TrimPrefix(host, "unix://")
			if socket != "/var/run/docker.sock" {
				return fmt.Errorf("rootless docker is not supported (DOCKER_HOST=%s)", host)
			}
		} else {
			return fmt.Errorf("rootless docker is not supported (DOCKER_HOST=%s)", host)
		}
	}
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return nil
	}
	if rootlessDockerSocketExists() {
		return fmt.Errorf("rootless docker is not supported; enable rootful docker on the host")
	}
	return nil
}

func rootlessDockerSocketExists() bool {
	entries, err := os.ReadDir("/run/user")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		socket := filepath.Join("/run/user", entry.Name(), "docker.sock")
		info, err := os.Stat(socket)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSocket != 0 {
			return true
		}
	}
	return false
}
