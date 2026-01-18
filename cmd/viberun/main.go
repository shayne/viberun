// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/shayne/viberun/internal/agents"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/sshcmd"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
	"github.com/shayne/yargs"
)

func main() {
	if err := runCLI(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runCLI() error {
	args := ensureRunSubcommand(os.Args[1:])
	handlers := map[string]yargs.SubcommandHandler{
		"run":       handleRunCommand,
		"config":    handleConfigCommand,
		"bootstrap": handleBootstrapCommand,
	}
	if err := yargs.RunSubcommands(context.Background(), args, helpConfig, struct{}{}, handlers); err != nil {
		if errors.Is(err, yargs.ErrShown) {
			return nil
		}
		return err
	}
	return nil
}

type runFlags struct {
	Agent  string `flag:"agent" help:"agent provider to run (codex, claude, gemini, ampcode, opencode, or npx:<pkg>/uvx:<pkg>)"`
	Delete bool   `flag:"delete" help:"delete the app and snapshots"`
	Yes    bool   `flag:"yes" short:"y" help:"skip confirmation prompts"`
}

type runArgs struct {
	Target string `pos:"0" help:"app or app@host"`
	Action string `pos:"1?" help:"snapshot|snapshots|restore|shell"`
	Value  string `pos:"2?" help:"snapshot name for restore"`
}

type configFlags struct {
	Host        string   `flag:"host" help:"set default host (alias for --default-host)"`
	DefaultHost string   `flag:"default-host" help:"set default host"`
	Agent       string   `flag:"agent" help:"set default agent provider"`
	SetHosts    []string `flag:"set-host" help:"set host alias mapping as alias=host (repeatable)"`
}

type bootstrapFlags struct {
	Local      bool   `flag:"local" help:"install server from local build instead of GitHub release"`
	LocalPath  string `flag:"local-path" help:"install server from a local binary at this path"`
	LocalImage bool   `flag:"local-image" help:"build and load the container image from the local Docker daemon"`
}

type bootstrapArgs struct {
	Host string `pos:"0?" help:"host to bootstrap"`
}

var helpConfig = yargs.HelpConfig{
	Command: yargs.CommandInfo{
		Name:        "viberun",
		Description: "CLI-first agent app host",
		Examples: []string{
			"viberun myapp",
			"viberun myapp snapshot",
			"viberun myapp restore latest",
			"viberun myapp shell",
			"viberun config --host myhost --agent codex",
			"viberun bootstrap root@1.2.3.4",
		},
	},
	SubCommands: map[string]yargs.SubCommandInfo{
		"run": {
			Name:        "run",
			Description: "Run or manage an app session",
			Usage:       "<app> [snapshot|snapshots|restore <snapshot>|shell]",
			Hidden:      true,
		},
		"config": {
			Name:        "config",
			Description: "Show or update the local configuration",
		},
		"bootstrap": {
			Name:        "bootstrap",
			Description: "Install or update the host-side server and image",
			Usage:       "[<host>]",
		},
	},
}

func ensureRunSubcommand(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if isHelpFlag(args[0]) {
		return args
	}
	cmd := firstNonFlag(args)
	if cmd == "" {
		return args
	}
	if cmd == "help" {
		return []string{"--help"}
	}
	switch cmd {
	case "run", "config", "bootstrap":
		return args
	default:
		return append([]string{"run"}, args...)
	}
}

func isHelpFlag(value string) bool {
	switch strings.TrimSpace(value) {
	case "-h", "--help", "--help-llm":
		return true
	default:
		return false
	}
}

func firstNonFlag(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

func handleRunCommand(_ context.Context, args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, runFlags, runArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	return runApp(result.SubCommandFlags, result.Args)
}

func handleConfigCommand(_ context.Context, args []string) error {
	handleConfig(args)
	return nil
}

func handleBootstrapCommand(_ context.Context, args []string) error {
	handleBootstrap(args)
	return nil
}

func handleConfig(args []string) {
	result, err := yargs.ParseAndHandleHelp[struct{}, configFlags, struct{}](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	flags := result.SubCommandFlags
	cfg, path, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if configFlagsEmpty(flags) {
		showConfig(cfg, path)
		return
	}

	updated := false
	resolvedHost := strings.TrimSpace(flags.DefaultHost)
	if strings.TrimSpace(flags.Host) != "" && resolvedHost != "" && strings.TrimSpace(flags.Host) != resolvedHost {
		fmt.Fprintln(os.Stderr, "conflicting --host and --default-host values")
		os.Exit(2)
	}
	if strings.TrimSpace(flags.Host) != "" {
		resolvedHost = strings.TrimSpace(flags.Host)
	}
	if resolvedHost != "" {
		cfg.DefaultHost = resolvedHost
		updated = true
	}
	if strings.TrimSpace(flags.Agent) != "" {
		cfg.AgentProvider = strings.TrimSpace(flags.Agent)
		updated = true
	}
	if len(flags.SetHosts) > 0 {
		if cfg.Hosts == nil {
			cfg.Hosts = map[string]string{}
		}
		for _, entry := range flags.SetHosts {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "invalid host mapping %q (expected alias=host)\n", entry)
				os.Exit(2)
			}
			alias := strings.TrimSpace(parts[0])
			host := strings.TrimSpace(parts[1])
			if alias == "" || host == "" {
				fmt.Fprintf(os.Stderr, "invalid host mapping %q (expected alias=host)\n", entry)
				os.Exit(2)
			}
			cfg.Hosts[alias] = host
		}
		updated = true
	}
	if !updated {
		showConfig(cfg, path)
		return
	}

	if err := config.Save(path, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote config to %s\n", path)
}

func showConfig(cfg config.Config, path string) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to format config: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "Config path: %s\n%s\n", path, string(data))
}

func handleBootstrap(args []string) {
	result, err := yargs.ParseAndHandleHelp[struct{}, bootstrapFlags, bootstrapArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	flags := result.SubCommandFlags
	hostArg := strings.TrimSpace(result.Args.Host)

	cfg, path, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	resolved, err := target.ResolveHost(hostArg, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid host: %v\n", err)
		os.Exit(2)
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		fmt.Fprintln(os.Stderr, "ssh is required but was not found in PATH")
		os.Exit(1)
	}

	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !tty {
		fmt.Fprintln(os.Stderr, "bootstrap may require sudo; run from a terminal if you are prompted for a password")
	}
	ui := tui.NewProgress(os.Stdout, tty, "bootstrap", resolved.Host)
	ui.Start()
	defer ui.Stop()

	env := []string{}
	for _, key := range []string{"VIBERUN_IMAGE", "VIBERUN_SERVER_VERSION", "VIBERUN_SERVER_REPO"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env = append(env, key+"="+value)
		}
	}
	localBootstrap := flags.Local
	localPath := strings.TrimSpace(flags.LocalPath)
	localImage := flags.LocalImage
	if strings.TrimSpace(localPath) != "" {
		localBootstrap = true
	}
	if isDevRun() {
		if !localBootstrap && strings.TrimSpace(localPath) == "" {
			localBootstrap = true
		}
		if !localImage {
			localImage = true
		}
	}
	if localBootstrap {
		ui.Step("Stage server binary")
		remotePath, err := stageLocalServerBinary(resolved.Host, localPath)
		if err != nil {
			ui.Fail(err.Error())
			fmt.Fprintf(os.Stderr, "failed to stage local server binary: %v\n", err)
			os.Exit(1)
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
			fmt.Fprintf(os.Stderr, "failed to stage local image: %v\n", err)
			os.Exit(1)
		}
		ui.Resume()
		ui.Done("")
		env = append(env, "VIBERUN_SKIP_IMAGE_PULL=1")
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

	ui.Step("Run bootstrap")
	ui.Suspend()
	if err := cmd.Run(); err != nil {
		ui.Resume()
		ui.Fail(err.Error())
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "failed to start ssh: %v\n", err)
		os.Exit(1)
	}
	ui.Resume()
	ui.Done("")
	if strings.TrimSpace(cfg.DefaultHost) == "" && strings.TrimSpace(hostArg) != "" {
		cfg.DefaultHost = hostArg
		if err := config.Save(path, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Bootstrap complete, but failed to save default host: %v\n", err)
			fmt.Fprintf(os.Stderr, "Run `viberun config --host %s` to set it manually.\n", hostArg)
		} else {
			fmt.Fprintf(os.Stdout, "default host set to %s\n", hostArg)
		}
	}
	fmt.Fprintln(os.Stdout, "Bootstrap complete.")
}

func configFlagsEmpty(flags configFlags) bool {
	return strings.TrimSpace(flags.Host) == "" &&
		strings.TrimSpace(flags.DefaultHost) == "" &&
		strings.TrimSpace(flags.Agent) == "" &&
		len(flags.SetHosts) == 0
}

func runApp(flags runFlags, args runArgs) error {
	targetArg := strings.TrimSpace(args.Target)
	if targetArg == "" {
		exitUsage("Usage: viberun [--agent provider] <app> | viberun [--agent provider] <app>@<host> | viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun [--agent provider] <app> restore <snapshot> | viberun <app> shell | viberun <app> --delete [-y] | viberun bootstrap [<host>] | viberun config [options]")
	}

	actionArgs := []string{}
	action := strings.TrimSpace(args.Action)
	value := strings.TrimSpace(args.Value)
	if action != "" {
		switch action {
		case "snapshot":
			if value != "" {
				exitUsage("Usage: viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun <app> shell")
			}
			actionArgs = []string{"snapshot"}
		case "snapshots":
			if value != "" {
				exitUsage("Usage: viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun <app> shell")
			}
			actionArgs = []string{"snapshots"}
		case "shell":
			if value != "" {
				exitUsage("Usage: viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun <app> shell")
			}
			actionArgs = []string{"shell"}
		case "restore":
			if value == "" {
				exitUsage("Usage: viberun [--agent provider] <app> restore <snapshot>")
			}
			actionArgs = []string{"restore", value}
		default:
			exitUsage("Usage: viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun [--agent provider] <app> restore <snapshot> | viberun <app> shell")
		}
	}
	if flags.Delete {
		if len(actionArgs) != 0 {
			exitUsage("Usage: viberun [--delete] <app> | viberun [--agent provider] <app> snapshot | viberun [--agent provider] <app> snapshots | viberun [--agent provider] <app> restore <snapshot> | viberun <app> shell")
		}
		if !flags.Yes {
			if !promptDelete(targetArg) {
				fmt.Fprintln(os.Stdout, "delete cancelled")
				return nil
			}
		}
		actionArgs = []string{"delete"}
	}

	interactive := len(actionArgs) == 0 || (len(actionArgs) == 1 && actionArgs[0] == "shell")
	tty := interactive && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if strings.TrimSpace(flags.Agent) == "" && strings.TrimSpace(cfg.AgentProvider) == "" && tty {
		selection, err := tui.SelectDefaultAgent(os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if strings.TrimSpace(selection) != "" {
			cfg.AgentProvider = selection
			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}
	}

	resolved, err := target.Resolve(targetArg, cfg)
	if err != nil {
		exitUsage(fmt.Sprintf("invalid target: %v", err))
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}

	agentProvider := cfg.AgentProvider
	if strings.TrimSpace(agentProvider) == "" {
		agentProvider = agents.DefaultProvider()
	}
	if strings.TrimSpace(flags.Agent) != "" {
		agentProvider = strings.TrimSpace(flags.Agent)
	}
	agentSpec, err := agents.Resolve(agentProvider)
	if err != nil {
		return err
	}
	agentProvider = agentSpec.Provider
	if interactive && !tty {
		return fmt.Errorf("interactive sessions require a TTY; run from a terminal or use snapshot/restore commands")
	}
	extraEnv := map[string]string{}
	if tty {
		if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
			extraEnv["COLORTERM"] = colorTerm
		}
	}
	if interactive && tty {
		exists, err := remoteContainerExists(resolved, agentProvider)
		if err != nil {
			return err
		}
		if !exists {
			if !promptCreateLocal(resolved.App) {
				fmt.Fprintln(os.Stderr, "aborted")
				os.Exit(1)
			}
			extraEnv["VIBERUN_AUTO_CREATE"] = "1"
			localAuth, details, err := discoverLocalAuth(agentSpec.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "auth discovery failed: %v\n", err)
			} else if localAuth != nil && promptCopyAuth(resolved.App, agentSpec.Label, details) {
				bundle, err := stageAuthBundle(resolved.Host, localAuth)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to stage auth: %v\n", err)
				} else if encoded, err := encodeAuthBundle(bundle); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode auth: %v\n", err)
				} else if encoded != "" {
					extraEnv["VIBERUN_AUTH_BUNDLE"] = encoded
				}
			}
		}
	}
	var openServer *http.Server
	var remoteSocket *sshcmd.RemoteSocketForward
	var forwardCmd *exec.Cmd
	cleanup := func() {
		if openServer != nil {
			_ = openServer.Close()
			openServer = nil
		}
		if forwardCmd != nil {
			stopLocalForward(forwardCmd)
			forwardCmd = nil
		}
	}
	defer cleanup()
	if interactive {
		socketPath := newXdgOpenSocketPath(resolved.App)
		if !isLocalHost(resolved.Host) {
			removeRemoteSocket(resolved.Host, socketPath)
		}
		server, port, err := startOpenListener()
		if err != nil {
			return fmt.Errorf("failed to start xdg-open listener: %w", err)
		}
		openServer = server
		extraEnv["VIBERUN_XDG_OPEN_SOCKET"] = socketPath
		remoteSocket = &sshcmd.RemoteSocketForward{
			RemotePath: socketPath,
			LocalHost:  "localhost",
			LocalPort:  port,
		}
	}
	remoteArgs := sshcmd.RemoteArgs(resolved.App, agentProvider, actionArgs, extraEnv)
	if interactive && !isLocalHost(resolved.Host) {
		hostPort, err := resolveHostPort(resolved, agentProvider)
		if err != nil {
			return err
		}
		if err := ensureLocalPortAvailable(hostPort); err != nil {
			return err
		}
		forwardCmd, err = startLocalForward(resolved.Host, &sshcmd.LocalForward{
			LocalPort:  hostPort,
			RemoteHost: "localhost",
			RemotePort: hostPort,
		})
		if err != nil {
			return err
		}
	}

	sshArgs := sshcmd.BuildArgsWithForwards(resolved.Host, remoteArgs, tty, nil, remoteSocket)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		cleanup()
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to start ssh: %w", err)
	}
	return nil
}

func exitUsage(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}

func bootstrapScript() string {
	return `set -euo pipefail

if [ ! -f /etc/os-release ]; then
  echo "missing /etc/os-release; cannot verify OS" >&2
  exit 1
fi

. /etc/os-release
if [ "${ID:-}" != "ubuntu" ]; then
  echo "unsupported OS: ${ID:-unknown}; expected ubuntu" >&2
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  if ! need_cmd sudo; then
    echo "sudo is required to bootstrap as a non-root user" >&2
    exit 1
  fi
  SUDO="sudo"
  if ! sudo -n true 2>/dev/null; then
    echo "sudo password may be required during bootstrap" >&2
  fi
fi

if ! need_cmd curl && ! need_cmd wget; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y curl ca-certificates
fi

if ! need_cmd docker; then
  if need_cmd curl; then
    curl -fsSL https://get.docker.com | $SUDO sh
  else
    wget -qO- https://get.docker.com | $SUDO sh
  fi
fi

if need_cmd systemctl; then
  $SUDO systemctl enable --now docker
fi

pull_image() {
  if [ -n "${VIBERUN_SKIP_IMAGE_PULL:-}" ]; then
    return 0
  fi
  image="$1"
  if [ -z "$image" ]; then
    return 0
  fi
  if ! $SUDO docker pull "$image"; then
    echo "warning: failed to pull image $image" >&2
    return 0
  fi
  $SUDO docker tag "$image" "viberun:latest" || true
}

if [ "$(id -u)" -ne 0 ]; then
  if ! getent group docker >/dev/null 2>&1; then
    $SUDO groupadd docker
  fi
  if ! id -nG "$USER" | tr ' ' '\n' | grep -qx docker; then
    $SUDO usermod -aG docker "$USER"
    echo "added $USER to docker group; run 'newgrp docker' or reconnect to apply" >&2
  fi
fi

VIBERUN_SERVER_REPO="${VIBERUN_SERVER_REPO:-shayne/viberun}"
VIBERUN_SERVER_VERSION="${VIBERUN_SERVER_VERSION:-latest}"
VIBERUN_SERVER_INSTALL_DIR="${VIBERUN_SERVER_INSTALL_DIR:-/usr/local/bin}"
VIBERUN_SERVER_BIN="${VIBERUN_SERVER_BIN:-viberun-server}"
VIBERUN_IMAGE="${VIBERUN_IMAGE:-}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64)
    arch="amd64"
    ;;
  arm64|aarch64)
    arch="arm64"
    ;;
  *)
    echo "unsupported architecture: $arch_raw" >&2
    exit 1
    ;;
esac

if [ "$os" != "linux" ]; then
  echo "unsupported OS: $os; expected linux" >&2
  exit 1
fi

if [ -z "$VIBERUN_IMAGE" ]; then
  if [ "$VIBERUN_SERVER_VERSION" = "latest" ]; then
    VIBERUN_IMAGE="ghcr.io/${VIBERUN_SERVER_REPO}/viberun:latest"
  else
    VIBERUN_IMAGE="ghcr.io/${VIBERUN_SERVER_REPO}/viberun:${VIBERUN_SERVER_VERSION}"
  fi
fi

if need_cmd docker; then
  pull_image "$VIBERUN_IMAGE"
fi

asset="viberun-server-${os}-${arch}.tar.gz"
sha="${asset}.sha256"
local_path="${VIBERUN_SERVER_LOCAL_PATH:-}"
tmp_dir=""
binary_path=""

cleanup() {
  if [ -n "${tmp_dir:-}" ]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

if [ -n "$local_path" ]; then
  if [ ! -f "$local_path" ]; then
    echo "missing local server binary at $local_path" >&2
    exit 1
  fi
  case "$local_path" in
    *.tar.gz)
      if ! need_cmd tar; then
        echo "tar is required" >&2
        exit 1
      fi
      tmp_dir="$(mktemp -d)"
      tar -xzf "$local_path" -C "$tmp_dir"
      binary_path="$tmp_dir/viberun-server-${os}-${arch}"
      if [ ! -f "$binary_path" ]; then
        echo "missing extracted binary: viberun-server-${os}-${arch}" >&2
        exit 1
      fi
      ;;
    *)
      binary_path="$local_path"
      ;;
  esac
else
  if [ "$VIBERUN_SERVER_VERSION" = "latest" ]; then
    download_url="https://github.com/${VIBERUN_SERVER_REPO}/releases/latest/download/${asset}"
    sha_url="https://github.com/${VIBERUN_SERVER_REPO}/releases/latest/download/${sha}"
  else
    version="$VIBERUN_SERVER_VERSION"
    case "$version" in
      v*)
        ;;
      *)
        version="v$version"
        ;;
    esac
    download_url="https://github.com/${VIBERUN_SERVER_REPO}/releases/download/${version}/${asset}"
    sha_url="https://github.com/${VIBERUN_SERVER_REPO}/releases/download/${version}/${sha}"
  fi

  tmp_dir="$(mktemp -d)"
  if need_cmd curl; then
    curl -fsSL "$download_url" -o "$tmp_dir/$asset"
    curl -fsSL "$sha_url" -o "$tmp_dir/$sha"
  else
    wget -qO "$tmp_dir/$asset" "$download_url"
    wget -qO "$tmp_dir/$sha" "$sha_url"
  fi

  normalize_sha() {
    awk '{
      fname=$NF
      star=""
      if (fname ~ /^\*/) { star="*"; fname=substr(fname,2) }
      sub(/^.*\//, "", fname)
      print $1 "  " star fname
    }'
  }

  sha_check="$sha"
  if grep -q '/' "$tmp_dir/$sha"; then
    normalize_sha < "$tmp_dir/$sha" > "$tmp_dir/${sha}.normalized"
    sha_check="${sha}.normalized"
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmp_dir" && sha256sum -c "$sha_check")
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$tmp_dir" && shasum -a 256 -c "$sha_check")
  else
    echo "sha256sum or shasum is required" >&2
    exit 1
  fi

  if ! need_cmd tar; then
    echo "tar is required" >&2
    exit 1
  fi
  tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"
  binary_path="$tmp_dir/viberun-server-${os}-${arch}"
  if [ ! -f "$binary_path" ]; then
    echo "missing extracted binary: viberun-server-${os}-${arch}" >&2
    exit 1
  fi
fi

$SUDO install -m 0755 "$binary_path" "$VIBERUN_SERVER_INSTALL_DIR/$VIBERUN_SERVER_BIN"
`
}

func bootstrapCommand(script string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return "echo " + shellQuote(encoded) + " | base64 -d | bash"
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func resolveHostPort(resolved target.Resolved, agentProvider string) (int, error) {
	remoteArgs := sshcmd.RemoteArgs(resolved.App, agentProvider, []string{"port"}, nil)
	sshArgs := sshcmd.BuildArgs(resolved.Host, remoteArgs, false)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return 0, fmt.Errorf("failed to resolve host port: %s", trimmed)
	}
	portText := strings.TrimSpace(string(output))
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("unexpected host port response: %q", portText)
	}
	return port, nil
}

func remoteContainerExists(resolved target.Resolved, agentProvider string) (bool, error) {
	remoteArgs := sshcmd.RemoteArgs(resolved.App, agentProvider, []string{"exists"}, nil)
	sshArgs := sshcmd.BuildArgs(resolved.Host, remoteArgs, false)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return false, fmt.Errorf("failed to check container: %s", trimmed)
	}
	text := strings.TrimSpace(strings.ToLower(string(output)))
	return text == "true" || text == "yes" || text == "1", nil
}

func ensureLocalPortAvailable(port int) error {
	if port <= 0 {
		return fmt.Errorf("invalid host port %d", port)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("localhost port %d is unavailable: %v", port, err)
	}
	_ = listener.Close()
	return nil
}

func isLocalHost(host string) bool {
	normalized := strings.TrimSpace(host)
	if normalized == "" {
		return false
	}
	if at := strings.LastIndex(normalized, "@"); at >= 0 {
		normalized = normalized[at+1:]
	}
	normalized = strings.TrimSpace(normalized)
	if strings.HasPrefix(normalized, "[") {
		if end := strings.Index(normalized, "]"); end > 0 {
			normalized = normalized[1:end]
		}
	} else if colon := strings.Index(normalized, ":"); colon > 0 {
		normalized = normalized[:colon]
	}
	normalized = strings.ToLower(strings.TrimSpace(normalized))
	switch normalized {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func newXdgOpenSocketPath(app string) string {
	const dir = "/tmp/viberun-open"
	const suffix = ".sock"
	cleaned := strings.TrimSpace(app)
	if cleaned != "" {
		var b strings.Builder
		for _, r := range cleaned {
			switch {
			case r >= 'a' && r <= 'z':
				b.WriteRune(r)
			case r >= 'A' && r <= 'Z':
				b.WriteRune(r)
			case r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == '-' || r == '_':
				b.WriteRune(r)
			default:
				b.WriteRune('-')
			}
		}
		slug := strings.Trim(b.String(), "-")
		if slug != "" {
			return dir + "/" + slug + suffix
		}
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return dir + "/" + hex.EncodeToString(buf) + suffix
	}
	return fmt.Sprintf("%s/%d-%d%s", dir, os.Getpid(), time.Now().UnixNano(), suffix)
}

func removeRemoteSocket(host string, path string) {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(path) == "" {
		return
	}
	dir := filepath.Dir(path)
	remoteArgs := []string{"sh", "-lc", shellQuote("mkdir -p " + dir + " && rm -f " + path)}
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove xdg-open socket %s: %v\n", path, err)
	}
}

func startOpenListener() (*http.Server, int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/open" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			raw := strings.TrimSpace(r.Form.Get("url"))
			cleaned, err := validateOpenURL(raw)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := openURL(cleaned); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return server, port, nil
}

func validateOpenURL(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "", fmt.Errorf("missing url")
	}
	if strings.ContainsAny(cleaned, "\r\n\t") {
		return "", fmt.Errorf("invalid url")
	}
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return cleaned, nil
	default:
		return "", fmt.Errorf("unsupported url scheme")
	}
}

func openURL(raw string) error {
	switch runtime.GOOS {
	case "darwin":
		if path, err := exec.LookPath("open"); err == nil {
			return exec.Command(path, raw).Start()
		}
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw).Start()
	default:
		if path, err := exec.LookPath("xdg-open"); err == nil {
			return exec.Command(path, raw).Start()
		}
		if path, err := exec.LookPath("open"); err == nil {
			return exec.Command(path, raw).Start()
		}
	}
	return fmt.Errorf("no opener available")
}

func promptDelete(app string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Delete %s and all snapshots? [y/N]: ", app)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func promptCreateLocal(app string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "App %s does not exist. Create? [Y/n]: ", app)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}

func isDevRun() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return strings.Contains(os.Args[0], "go-build")
	}
	if info.Path == "command-line-arguments" {
		return true
	}
	return strings.Contains(os.Args[0], "go-build")
}

func stageLocalImage(host string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required to build the image locally")
	}
	_, arch, err := detectRemotePlatform(host)
	if err != nil {
		return err
	}
	tag := "viberun:dev"
	buildCmd := exec.Command("docker", "build", "--platform", "linux/"+arch, "-t", tag, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return err
	}
	sshArgs := sshcmd.BuildArgs(host, []string{"docker", "load"}, false)
	loadCmd := exec.Command("ssh", sshArgs...)
	loadCmd.Env = normalizedSshEnv()
	loadCmd.Stdout = os.Stdout
	loadCmd.Stderr = os.Stderr
	loadIn, err := loadCmd.StdinPipe()
	if err != nil {
		return err
	}
	saveCmd := exec.Command("docker", "save", tag)
	saveCmd.Stdout = loadIn
	saveCmd.Stderr = os.Stderr
	if err := loadCmd.Start(); err != nil {
		_ = loadIn.Close()
		return err
	}
	if err := saveCmd.Run(); err != nil {
		_ = loadIn.Close()
		_ = loadCmd.Wait()
		return err
	}
	_ = loadIn.Close()
	if err := loadCmd.Wait(); err != nil {
		return err
	}
	tagArgs := sshcmd.BuildArgs(host, []string{"docker", "tag", tag, "viberun:latest"}, false)
	tagCmd := exec.Command("ssh", tagArgs...)
	tagCmd.Env = normalizedSshEnv()
	tagCmd.Stdout = os.Stdout
	tagCmd.Stderr = os.Stderr
	return tagCmd.Run()
}

func stageLocalServerBinary(host string, localPath string) (string, error) {
	path := strings.TrimSpace(localPath)
	if path == "" {
		osName, arch, err := detectRemotePlatform(host)
		if err != nil {
			return "", err
		}
		if osName != "linux" {
			return "", fmt.Errorf("unsupported remote OS: %s", osName)
		}
		tmpFile, err := os.CreateTemp("", "viberun-server-")
		if err != nil {
			return "", err
		}
		tmpPath := tmpFile.Name()
		_ = tmpFile.Close()
		buildCmd := exec.Command("go", "build", "-o", tmpPath, "./cmd/viberun-server")
		buildCmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS=linux",
			"GOARCH="+arch,
		)
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return "", fmt.Errorf("build failed: %w", err)
		}
		path = tmpPath
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("local binary not found: %w", err)
	}
	remotePath := fmt.Sprintf("/tmp/viberun-server-%d", time.Now().UnixNano())
	if err := uploadFileOverSSH(host, path, remotePath); err != nil {
		return "", err
	}
	return remotePath, nil
}

func detectRemotePlatform(host string) (string, string, error) {
	osName, err := sshOutput(host, []string{"uname", "-s"})
	if err != nil {
		return "", "", err
	}
	archRaw, err := sshOutput(host, []string{"uname", "-m"})
	if err != nil {
		return "", "", err
	}
	arch, err := normalizeArch(archRaw)
	if err != nil {
		return "", "", err
	}
	return strings.ToLower(osName), arch, nil
}

func normalizeArch(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "x86_64", "amd64":
		return "amd64", nil
	case "arm64", "aarch64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", raw)
	}
}

func sshOutput(host string, remoteArgs []string) (string, error) {
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return "", fmt.Errorf("ssh failed: %s", trimmed)
	}
	return strings.TrimSpace(string(out)), nil
}

func uploadFileOverSSH(host string, localPath string, remotePath string) error {
	remote := []string{"bash", "-lc", "cat > " + shellQuote(remotePath)}
	sshArgs := sshcmd.BuildArgs(host, remote, false)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd.Stdin = file
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func startLocalForward(host string, forward *sshcmd.LocalForward) (*exec.Cmd, error) {
	args := sshcmd.BuildPortForwardArgs(host, forward)
	cmd := exec.Command("ssh", args...)
	cmd.Env = normalizedSshEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start port forward: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		waitErr := cmd.Wait()
		if waitErr != nil {
			return nil, fmt.Errorf("port forward failed: %w", waitErr)
		}
		return nil, fmt.Errorf("port forward exited unexpectedly")
	}
	return cmd, nil
}

func stopLocalForward(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(750 * time.Millisecond):
		_ = cmd.Process.Kill()
		<-done
	}
}

func normalizedSshEnv() []string {
	termValue := normalizeTermForSsh(os.Getenv("TERM"))
	if termValue == "" || termValue == os.Getenv("TERM") {
		return os.Environ()
	}
	return replaceEnv(os.Environ(), "TERM", termValue)
}

func normalizeTermForSsh(termValue string) string {
	return strings.TrimSpace(termValue)
}

func replaceEnv(env []string, key string, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}
