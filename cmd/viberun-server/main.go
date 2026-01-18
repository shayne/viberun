// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/shayne/viberun/internal/server"
	"github.com/shayne/yargs"
)

const defaultImage = "viberun:latest"

type serverFlags struct {
	Agent string `flag:"agent" help:"agent provider to run (codex, claude, gemini)"`
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || hasHelpFlag(args) {
		fmt.Fprintln(os.Stderr, "Usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|shell|port|delete|exists]")
		os.Exit(2)
	}
	result, err := yargs.ParseFlags[serverFlags](args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	if len(result.Args) < 1 || len(result.Args) > 3 {
		fmt.Fprintln(os.Stderr, "Usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|shell|port|delete|exists]")
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
	if agentProvider == "" {
		agentProvider = "codex"
	}
	agentArgs, err := agentCommand(agentProvider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid agent provider: %v\n", err)
		os.Exit(2)
	}
	sessionName := "viberun-agent"
	if action == "shell" {
		agentArgs = []string{"/bin/bash"}
		sessionName = "viberun-shell"
	}
	agentArgs = tmuxSessionArgs(sessionName, agentArgs)

	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Fprintln(os.Stderr, "docker is required but was not found in PATH")
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
		ref, err := createSnapshot(containerName, app)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stdout, "Snapshot created: %s\n", ref)
		return
	}
	if action == "snapshots" {
		tags, err := listSnapshots(app)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list snapshots: %v\n", err)
			os.Exit(1)
		}
		if len(tags) == 0 {
			fmt.Fprintf(os.Stdout, "No snapshots found for %s\n", app)
			return
		}
		fmt.Fprintf(os.Stdout, "Snapshots for %s:\n", app)
		for _, tag := range tags {
			fmt.Fprintf(os.Stdout, "  %s %s\n", app, tag)
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

	if !exists {
		if !autoCreateEnabled() {
			if !promptCreate(app) {
				fmt.Fprintln(os.Stderr, "aborted")
				os.Exit(1)
			}
		}

		if err := dockerRun(containerName, app, port); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create container: %v\n", err)
			os.Exit(1)
		}
	} else {
		running, err := containerRunning(containerName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to check container state: %v\n", err)
			os.Exit(1)
		}
		if !running {
			if err := dockerStart(containerName); err != nil {
				fmt.Fprintf(os.Stderr, "failed to start container: %v\n", err)
				os.Exit(1)
			}
		}
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

	authBundle, err := loadAuthBundleFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load auth bundle: %v\n", err)
		os.Exit(1)
	}
	if !exists && authBundle != nil {
		if err := applyAuthBundle(containerName, authBundle); err != nil {
			fmt.Fprintf(os.Stderr, "failed to apply auth bundle: %v\n", err)
			os.Exit(1)
		}
	}

	if err := dockerExec(containerName, agentArgs); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "failed to exec shell: %v\n", err)
		os.Exit(1)
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
	return "", nil, fmt.Errorf("usage: viberun-server [--agent provider] <app> [snapshot|snapshots|restore <snapshot>|shell|port|delete|exists]")
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
	args := dockerRunArgs(name, app, port, defaultImage)
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerStart(name string) error {
	cmd := exec.Command("docker", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerExec(name string, agentArgs []string) error {
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

func dockerExecArgs(name string, agentArgs []string, tty bool, env map[string]string) []string {
	args := []string{"exec", "-i"}
	if tty {
		args = append(args, "-t")
	}
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

func snapshotRepo(app string) string {
	return fmt.Sprintf("viberun-snapshot-%s", app)
}

func deleteApp(containerName string, app string, state *server.State, exists bool) (bool, error) {
	removed := false
	if exists {
		cmd := exec.Command("docker", "rm", "-f", containerName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return false, err
		}
	}
	tags, err := listSnapshots(app)
	if err != nil {
		return false, err
	}
	if len(tags) > 0 {
		repo := snapshotRepo(app)
		for _, tag := range tags {
			ref := fmt.Sprintf("%s:%s", repo, tag)
			cmd := exec.Command("docker", "rmi", "-f", ref)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return false, err
			}
		}
	}
	if state != nil {
		removed = state.RemoveApp(app)
	}
	return removed, nil
}

func createSnapshot(containerName string, app string) (string, error) {
	repo := snapshotRepo(app)
	tag := time.Now().UTC().Format("20060102-150405")
	ref := fmt.Sprintf("%s:%s", repo, tag)
	cmd := exec.Command("docker", "commit", containerName, ref)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return ref, nil
}

func resolveSnapshotRef(app string, name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	if normalized == "latest" {
		return latestSnapshotRef(app)
	}
	if strings.Contains(normalized, ":") {
		return normalized, nil
	}
	return fmt.Sprintf("%s:%s", snapshotRepo(app), normalized), nil
}

func listSnapshots(app string) ([]string, error) {
	repo := snapshotRepo(app)
	out, err := exec.Command("docker", "images", "--format", "{{.Tag}}", repo).Output()
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" || tag == "<none>" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func latestSnapshotRef(app string) (string, error) {
	repo := snapshotRepo(app)
	out, err := exec.Command("docker", "images", "--format", "{{.Tag}}", repo).Output()
	if err != nil {
		return "", err
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" || tag == "<none>" {
			continue
		}
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("no snapshots found for %s", app)
	}
	sort.Strings(tags)
	return fmt.Sprintf("%s:%s", repo, tags[len(tags)-1]), nil
}

func restoreSnapshot(containerName string, app string, port int, snapshotRef string) error {
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
	args := dockerRunArgs(containerName, app, port, snapshotRef)
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerRunArgs(name string, app string, port int, image string) []string {
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
	if socketPath, ok := xdgOpenSocketPath(); ok {
		args = append(args,
			"-v",
			fmt.Sprintf("%s:%s", socketPath, socketPath),
			"-e",
			fmt.Sprintf("VIBERUN_XDG_OPEN_SOCKET=%s", socketPath),
		)
	}
	args = append(args, image, "/usr/bin/s6-svscan", "/etc/services.d")
	return args
}

func agentCommand(provider string) ([]string, error) {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "", "codex":
		return []string{"codex"}, nil
	case "claude", "claude-code":
		return []string{"claude"}, nil
	case "gemini":
		return []string{"gemini"}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

func tmuxSessionArgs(session string, command []string) []string {
	if strings.TrimSpace(session) == "" {
		session = "viberun-session"
	}
	if len(command) == 0 {
		command = []string{"/bin/bash"}
	}
	args := []string{"tmux", "new-session", "-A", "-s", session}
	return append(args, command...)
}

func xdgOpenSocketPath() (string, bool) {
	socket := strings.TrimSpace(os.Getenv("VIBERUN_XDG_OPEN_SOCKET"))
	if socket == "" {
		return "", false
	}
	if waitForSocket(socket, 10, 100*time.Millisecond) {
		return socket, true
	}
	fmt.Fprintf(os.Stderr, "warning: VIBERUN_XDG_OPEN_SOCKET is set but socket not found at %s\n", socket)
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

func isSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}
