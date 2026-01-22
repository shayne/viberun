// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
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
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/pelletier/go-toml/v2"
	"github.com/shayne/viberun/internal/agents"
	"github.com/shayne/viberun/internal/clipboard"
	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/proxy"
	"github.com/shayne/viberun/internal/sshcmd"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
	"github.com/shayne/yargs"
)

func main() {
	if err := runCLI(); err != nil {
		reportCLIError(err)
	}
}

type usageError struct {
	message string
}

func (e usageError) Error() string {
	return e.message
}

type silentError struct {
	err error
}

func (e silentError) Error() string {
	return e.err.Error()
}

func (e silentError) Unwrap() error {
	return e.err
}

type missingHostError struct{}

func (missingHostError) Error() string {
	return "missing host configuration"
}

func reportCLIError(err error) {
	var usageErr usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(os.Stderr, usageErr.message)
		return
	}
	var hostErr missingHostError
	if errors.As(err, &hostErr) {
		printMissingHostMessage()
		return
	}
	var quietErr silentError
	if errors.As(err, &quietErr) {
		return
	}
	fmt.Fprintln(os.Stderr, err.Error())
}

func newUsageError(message string) error {
	return usageError{message: message}
}

func newSilentError(err error) error {
	if err == nil {
		return nil
	}
	return silentError{err: err}
}

var (
	version = "dev"
	commit  = ""
)

var devServerSync = struct {
	mu    sync.Mutex
	hosts map[string]bool
}{}

func runCLI() error {
	if shouldStartShell() {
		return runShell()
	}
	args := ensureRunSubcommand(normalizeArgs(os.Args[1:]))
	handlers := map[string]yargs.SubcommandHandler{
		"run":       handleRunCommand,
		"config":    handleConfigCommand,
		"bootstrap": handleBootstrapCommand,
		"proxy":     handleProxyCommand,
		"users":     handleUsersCommand,
		"wipe":      handleWipeCommand,
		"version":   handleVersionCommand,
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
	Agent        string `flag:"agent" help:"agent provider to run (codex, claude, gemini, ampcode, opencode, or npx:<pkg>/uvx:<pkg>)"`
	ForwardAgent bool   `flag:"forward-agent" short:"A" help:"forward local SSH agent into the container"`
	Delete       bool   `flag:"delete" help:"delete the app and snapshots (alias: --remove)"`
	Yes          bool   `flag:"yes" short:"y" help:"skip confirmation prompts"`
	Open         bool   `flag:"open" help:"open the app URL in a browser (url command only)"`
	MakePublic   bool   `flag:"make-public" help:"allow anyone to access the app URL (url command only)"`
	RequireLogin bool   `flag:"require-login" help:"require login for the app URL (url command only)"`
	DisableURL   bool   `flag:"disable" help:"disable the app URL (url command only)"`
	EnableURL    bool   `flag:"enable" help:"enable the app URL (url command only)"`
	SetDomain    string `flag:"set-domain" help:"set a full domain for the app URL (url command only)"`
	ResetDomain  bool   `flag:"reset-domain" help:"reset the app URL to the default domain (url command only)"`
}

type runArgs struct {
	Target string `pos:"0" help:"app or app@host"`
	Action string `pos:"1?" help:"snapshot|snapshots|restore|update|shell|url|users"`
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

type proxyArgs struct {
	Action string `pos:"0?" help:"setup"`
	Host   string `pos:"1?" help:"host to configure"`
}

type usersArgs struct {
	Action string `pos:"0?" help:"list|add|remove|set-password"`
	Host   string `pos:"1?" help:"host to configure"`
}

type wipeArgs struct {
	Host string `pos:"0?" help:"host to wipe"`
}

var helpConfig = yargs.HelpConfig{
	Command: yargs.CommandInfo{
		Name:        "viberun",
		Description: "CLI-first agent app host",
		Examples: []string{
			"viberun --help",
			"viberun help run",
			"viberun --version",
			"viberun <app>",
			"viberun <app> snapshot",
			"viberun <app> restore latest",
			"viberun <app> shell",
			"viberun <app> url",
			"viberun <app> users",
			"viberun config --host myhost --agent codex",
			"viberun bootstrap root@1.2.3.4",
			"viberun proxy setup",
			"viberun users list",
			"viberun wipe",
		},
	},
	SubCommands: map[string]yargs.SubCommandInfo{
		"run": {
			Name:        "run",
			Description: "Run or manage an app session",
			Usage:       "<app> [snapshot|snapshots|restore <snapshot>|update|shell|url]",
			Examples: []string{
				"viberun <app>",
				"viberun <app> snapshot",
				"viberun <app> restore latest",
				"viberun <app> shell",
				"viberun <app> url",
				"viberun <app> --delete -y",
			},
			Hidden: true,
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
		"proxy": {
			Name:        "proxy",
			Description: "Configure app URLs via the host proxy",
			Usage:       "setup [<host>]",
		},
		"users": {
			Name:        "users",
			Description: "Manage URL users",
			Usage:       "list | add --username <u> | remove --username <u> | set-password --username <u>",
		},
		"wipe": {
			Name:        "wipe",
			Description: "Wipe local config and all viberun state on a host",
			Usage:       "[<host>]",
		},
		"version": {
			Name:        "version",
			Description: "Show CLI version",
		},
	},
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if args[0] == "--version" {
		return append([]string{"version"}, args[1:]...)
	}
	if args[0] == "help" {
		return rewriteHelpArgs(args[1:])
	}
	return args
}

func rewriteHelpArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"--help"}
	}
	helpFlag := "--help"
	for _, arg := range args {
		if arg == "--help-llm" {
			helpFlag = "--help-llm"
			break
		}
	}
	if isHelpFlag(args[0]) || args[0] == "--help-llm" {
		return []string{helpFlag}
	}
	if isKnownCommand(args[0]) {
		return []string{args[0], helpFlag}
	}
	return []string{"run", helpFlag}
}

func isKnownCommand(value string) bool {
	switch value {
	case "run", "config", "bootstrap", "proxy", "users", "wipe", "version":
		return true
	default:
		return false
	}
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
	if isKnownCommand(cmd) {
		return args
	}
	return append([]string{"run"}, args...)
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
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			return ""
		}
		if strings.HasPrefix(arg, "-") {
			if strings.HasPrefix(arg, "--") {
				if strings.Contains(arg, "=") {
					continue
				}
				if consumesValue(arg) {
					skipNext = true
				}
			}
			continue
		}
		return arg
	}
	return ""
}

func consumesValue(flag string) bool {
	switch flag {
	case "--agent", "--host", "--default-host", "--set-host", "--local-path":
		return true
	default:
		return false
	}
}

func handleRunCommand(_ context.Context, args []string) error {
	args = normalizeRunArgs(args)
	accessFlags, err := parseAccessFlagValues(args)
	if err != nil {
		return err
	}
	result, err := yargs.ParseAndHandleHelp[struct{}, runFlags, runArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	return runApp(result.SubCommandFlags, result.Args, accessFlags)
}

func normalizeRunArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, len(args))
	for i, arg := range args {
		if arg == "--remove" {
			out[i] = "--delete"
			continue
		}
		if strings.HasPrefix(arg, "--remove=") {
			out[i] = "--delete=" + strings.TrimPrefix(arg, "--remove=")
			continue
		}
		out[i] = arg
	}
	return out
}

type boolFlagValue struct {
	set   bool
	value bool
}

type accessFlagValues struct {
	makePublic   boolFlagValue
	requireLogin boolFlagValue
}

func parseAccessFlagValues(args []string) (accessFlagValues, error) {
	makePublic, err := parseBoolFlagValue(args, "make-public")
	if err != nil {
		return accessFlagValues{}, err
	}
	requireLogin, err := parseBoolFlagValue(args, "require-login")
	if err != nil {
		return accessFlagValues{}, err
	}
	return accessFlagValues{makePublic: makePublic, requireLogin: requireLogin}, nil
}

func parseBoolFlagValue(args []string, name string) (boolFlagValue, error) {
	flag := "--" + name
	prefix := flag + "="
	var value boolFlagValue
	for _, arg := range args {
		if arg == flag {
			if value.set {
				return value, fmt.Errorf("%s specified more than once", flag)
			}
			value = boolFlagValue{set: true, value: true}
			continue
		}
		if strings.HasPrefix(arg, prefix) {
			if value.set {
				return value, fmt.Errorf("%s specified more than once", flag)
			}
			raw := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, prefix)))
			switch raw {
			case "true", "1":
				value = boolFlagValue{set: true, value: true}
			case "false", "0":
				value = boolFlagValue{set: true, value: false}
			default:
				return value, fmt.Errorf("invalid value for %s (expected true or false)", flag)
			}
		}
	}
	return value, nil
}

func handleConfigCommand(_ context.Context, args []string) error {
	return handleConfig(args)
}

func handleBootstrapCommand(_ context.Context, args []string) error {
	return handleBootstrap(args)
}

func handleVersionCommand(_ context.Context, args []string) error {
	_, err := yargs.ParseAndHandleHelp[struct{}, struct{}, struct{}](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, versionString())
	return nil
}

func handleConfig(args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, configFlags, struct{}](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}

	flags := result.SubCommandFlags
	cfg, path, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if configFlagsEmpty(flags) {
		return showConfig(cfg, path)
	}

	updated := false
	resolvedHost := strings.TrimSpace(flags.DefaultHost)
	if strings.TrimSpace(flags.Host) != "" && resolvedHost != "" && strings.TrimSpace(flags.Host) != resolvedHost {
		return newUsageError("conflicting --host and --default-host values")
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
				return newUsageError(fmt.Sprintf("invalid host mapping %q (expected alias=host)", entry))
			}
			alias := strings.TrimSpace(parts[0])
			host := strings.TrimSpace(parts[1])
			if alias == "" || host == "" {
				return newUsageError(fmt.Sprintf("invalid host mapping %q (expected alias=host)", entry))
			}
			cfg.Hosts[alias] = host
		}
		updated = true
	}
	if !updated {
		return showConfig(cfg, path)
	}

	if err := config.Save(path, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Fprintf(os.Stdout, "wrote config to %s\n", path)
	return nil
}

func showConfig(cfg config.Config, path string) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to format config: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Config path: %s\n%s\n", path, string(data))
	return nil
}

func handleBootstrap(args []string) error {
	if !isDevMode() && os.Getenv("VIBERUN_ALLOW_BOOTSTRAP") == "" {
		return fmt.Errorf("bootstrap is only available in development mode; run `viberun` to bootstrap interactively")
	}
	result, err := yargs.ParseAndHandleHelp[struct{}, bootstrapFlags, bootstrapArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	flags := result.SubCommandFlags
	hostArg := strings.TrimSpace(result.Args.Host)

	cfg, path, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved, err := target.ResolveHost(hostArg, cfg)
	if err != nil {
		return newUsageError(fmt.Sprintf("invalid host: %v", err))
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}

	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if !tty {
		fmt.Fprintln(os.Stderr, "bootstrap may require sudo; run from a terminal if you are prompted for a password")
	}
	ui := tui.NewProgress(os.Stdout, tty, "bootstrap", resolved.Host)
	ui.Start()
	defer ui.Stop()

	env := []string{}
	for _, key := range []string{"VIBERUN_IMAGE", "VIBERUN_PROXY_IMAGE", "VIBERUN_SERVER_VERSION", "VIBERUN_SERVER_REPO"} {
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
	if isDevRun() || isDevVersion() {
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
			return newSilentError(fmt.Errorf("failed to stage local server binary: %w", err))
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
			return newSilentError(fmt.Errorf("failed to stage local image: %w", err))
		}
		ui.Resume()
		ui.Done("")
		ui.Step("Build proxy image")
		ui.Suspend()
		if err := stageLocalProxyImage(resolved.Host); err != nil {
			ui.Resume()
			ui.Fail(err.Error())
			return newSilentError(fmt.Errorf("failed to stage local proxy image: %w", err))
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
			return newSilentError(exitErr)
		}
		return fmt.Errorf("failed to start ssh: %w", err)
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
	if tty {
		if err := runProxySetupFlow(resolved.Host, proxySetupOptions{updateArtifacts: true, showSkipHint: true}); err != nil {
			fmt.Fprintf(os.Stderr, "proxy setup failed: %v\n", err)
		}
	}
	fmt.Fprintln(os.Stdout, "Bootstrap complete.")
	return nil
}

func handleProxyCommand(_ context.Context, args []string) error {
	return handleProxy(args)
}

func handleUsersCommand(_ context.Context, args []string) error {
	return handleUsers(args)
}

func handleWipeCommand(_ context.Context, args []string) error {
	return handleWipe(args)
}

func handleProxy(args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, struct{}, proxyArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	action := strings.TrimSpace(result.Args.Action)
	if action == "" || action != "setup" {
		return newUsageError("Usage: viberun proxy setup [<host>]")
	}
	hostArg := strings.TrimSpace(result.Args.Host)

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved, err := target.ResolveHost(hostArg, cfg)
	if err != nil {
		return newUsageError(fmt.Sprintf("invalid host: %v", err))
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("proxy setup requires a TTY")
	}
	if err := runProxySetupFlow(resolved.Host, proxySetupOptions{updateArtifacts: true, forceSetup: true}); err != nil {
		return fmt.Errorf("proxy setup failed: %w", err)
	}
	return nil
}

type usersFlags struct {
	Username string `flag:"username" help:"username"`
}

func handleUsers(args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, struct{}, usersArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	action := strings.TrimSpace(result.Args.Action)
	if action == "" {
		return newUsageError("Usage: viberun users list|add|remove|set-password [<host>]")
	}
	hostArg := strings.TrimSpace(result.Args.Host)

	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	resolved, err := target.ResolveHost(hostArg, cfg)
	if err != nil {
		return newUsageError(fmt.Sprintf("invalid host: %v", err))
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}
	if err := ensureDevServerSynced(resolved.Host); err != nil {
		return fmt.Errorf("failed to sync dev server: %w", err)
	}

	switch action {
	case "list":
		if err := runRemoteUsersList(resolved.Host); err != nil {
			return fmt.Errorf("users list failed: %w", err)
		}
	case "add", "set-password", "remove":
		parsed, err := yargs.ParseFlags[usersFlags](args[1:])
		if err != nil {
			return err
		}
		username := strings.TrimSpace(parsed.Flags.Username)
		if username == "" {
			return newUsageError("username is required")
		}
		if action == "remove" {
			if err := runRemoteUsersRemove(resolved.Host, username); err != nil {
				return fmt.Errorf("users remove failed: %w", err)
			}
			return nil
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return fmt.Errorf("user management requires a TTY")
		}
		password, err := tui.PromptPassword(os.Stdin, os.Stdout, "Password")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		if action == "add" {
			if err := runRemoteUsersAdd(resolved.Host, username, password); err != nil {
				return fmt.Errorf("users add failed: %w", err)
			}
		} else {
			if err := runRemoteUsersSetPassword(resolved.Host, username, password); err != nil {
				return fmt.Errorf("users set-password failed: %w", err)
			}
		}
	default:
		return newUsageError("Usage: viberun users list|add|remove|set-password [<host>]")
	}
	return nil
}

func handleWipe(args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, struct{}, wipeArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("wipe requires a TTY")
	}
	cfg, _, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	resolved, err := target.ResolveHost(strings.TrimSpace(result.Args.Host), cfg)
	if err != nil {
		if errors.Is(err, target.ErrNoHostConfigured) {
			return missingHostError{}
		}
		return newUsageError(fmt.Sprintf("invalid host: %v", err))
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}
	wiped, err := runWipeFlow(resolved.Host, true)
	if err != nil {
		return fmt.Errorf("wipe failed: %w", err)
	}
	if !wiped {
		fmt.Fprintln(os.Stdout, "Wipe cancelled.")
	}
	return nil
}

func promptProxySetup() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, "Set up a public domain name? [y/N]: ")
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

type proxySetupOptions struct {
	updateArtifacts bool
	forceSetup      bool
	showSkipHint    bool
}

func runProxySetupFlow(host string, opts proxySetupOptions) error {
	configSummary, err := fetchRemoteProxyConfig(host)
	configured := err == nil && configSummary.Enabled && strings.TrimSpace(configSummary.BaseDomain) != "" && strings.TrimSpace(configSummary.PrimaryUser) != ""
	if err != nil && !opts.forceSetup {
		if !promptYesNoDefaultNo("Existing public domain settings could not be read. Continue with setup? [y/N]: ") {
			if opts.showSkipHint {
				fmt.Fprintln(os.Stdout, "Set up a public domain name later with: viberun proxy setup")
			}
			return nil
		}
	}

	shouldRunSetup := false
	editConfig := false
	domain := ""
	publicIP := ""
	username := ""
	password := ""

	if configured {
		printProxyConfigSummary(configSummary)
		editConfig = promptYesNoDefaultNo("Edit public domain settings? [y/N]: ")
		if editConfig || opts.updateArtifacts {
			shouldRunSetup = true
		}
		if !shouldRunSetup {
			return nil
		}
		domain = strings.TrimSpace(configSummary.BaseDomain)
		publicIP = strings.TrimSpace(configSummary.PublicIP)
		username = strings.TrimSpace(configSummary.PrimaryUser)
		if editConfig {
			if promptYesNoDefaultNo("Change public domain? [y/N]: ") {
				domain, err = tui.PromptProxyDomainWithDefault(os.Stdin, os.Stdout, "myapp.", domain)
				if err != nil {
					return err
				}
			}
			if strings.TrimSpace(publicIP) == "" {
				publicIP, err = fetchRemotePublicIP(host)
				if err != nil {
					return err
				}
			}
			if promptYesNoDefaultNo("Change public IP? [y/N]: ") {
				publicIP, err = tui.PromptProxyPublicIP(os.Stdin, os.Stdout, publicIP)
				if err != nil {
					return err
				}
			}
			if promptYesNoDefaultNo("Change primary user? [y/N]: ") {
				username, password, err = tui.PromptProxyAuth(os.Stdin, os.Stdout, username)
				if err != nil {
					return err
				}
			}
		} else if strings.TrimSpace(publicIP) == "" {
			publicIP, err = fetchRemotePublicIP(host)
			if err != nil {
				return err
			}
		}
	} else {
		if !opts.forceSetup && !promptProxySetup() {
			if opts.showSkipHint {
				fmt.Fprintln(os.Stdout, "Set up a public domain name later with: viberun proxy setup")
			}
			return nil
		}
		shouldRunSetup = true
		publicIP, err = fetchRemotePublicIP(host)
		if err != nil {
			return err
		}
		styler := newURLStyler(os.Stdout)
		fmt.Fprintf(os.Stdout, "%s %s\n", styler.label("Detected public IP:"), styler.ip(publicIP))
		domain, err = tui.PromptProxyDomain(os.Stdin, os.Stdout, "myapp.")
		if err != nil {
			return err
		}
		username, password, err = tui.PromptProxyAuth(os.Stdin, os.Stdout, "")
		if err != nil {
			return err
		}
	}

	proxyImage := ""
	if shouldRunSetup && opts.updateArtifacts && (isDevRun() || isDevVersion()) {
		if err := ensureDevServerSynced(host); err != nil {
			return err
		}
		if err := stageLocalProxyImage(host); err != nil {
			return err
		}
		proxyImage = devProxyImageTag()
	}

	domainChanged := !configured || strings.TrimSpace(domain) != strings.TrimSpace(configSummary.BaseDomain)
	if err := runRemoteProxySetup(host, domain, publicIP, username, password, proxyImage); err != nil {
		return err
	}
	if !configured || editConfig {
		printProxySetupInstructions(domain, publicIP, username)
	}
	if domainChanged {
		if err := promptRecreateApps(host); err != nil {
			return err
		}
	}
	return nil
}

func printProxySetupInstructions(domain string, publicIP string, username string) {
	styler := newURLStyler(os.Stdout)
	fmt.Fprintln(os.Stdout, styler.header("DNS setup:"))
	dnsLine := fmt.Sprintf("Create an A record for %s pointing to %s", styler.link("*."+domain), styler.ip(publicIP))
	fmt.Fprintf(os.Stdout, "  %s\n", styler.label(dnsLine))
	exampleLine := fmt.Sprintf("Example URL: %s", styler.link("https://myapp."+domain))
	fmt.Fprintf(os.Stdout, "  %s\n", styler.label(exampleLine))
	if strings.TrimSpace(username) != "" {
		fmt.Fprintln(os.Stdout, styler.header("Access:"))
		fmt.Fprintf(os.Stdout, "  %s\n", styler.label("All apps require login by default."))
		accessLine := fmt.Sprintf("Log in as %s with the password you just set.", styler.value(username))
		fmt.Fprintf(os.Stdout, "  %s\n", styler.label(accessLine))
	}
	fmt.Fprintln(os.Stdout, styler.header("Firewall:"))
	firewallLine := fmt.Sprintf("Ensure ports %s are open to the internet.", styler.value("80 and 443"))
	fmt.Fprintf(os.Stdout, "  %s\n", styler.label(firewallLine))
}

func printProxyConfigSummary(summary proxyConfigSummary) {
	styler := newURLStyler(os.Stdout)
	fmt.Fprintln(os.Stdout, styler.header("Current public domain setup:"))
	if strings.TrimSpace(summary.BaseDomain) != "" {
		fmt.Fprintf(os.Stdout, "  %s %s\n", styler.label("Domain:"), styler.value(summary.BaseDomain))
	}
	if strings.TrimSpace(summary.PublicIP) != "" {
		fmt.Fprintf(os.Stdout, "  %s %s\n", styler.label("Public IP:"), styler.ip(summary.PublicIP))
	}
	if strings.TrimSpace(summary.PrimaryUser) != "" {
		fmt.Fprintf(os.Stdout, "  %s %s\n", styler.label("Primary user:"), styler.value(summary.PrimaryUser))
	}
	if len(summary.Users) > 1 {
		fmt.Fprintf(os.Stdout, "  %s %s\n", styler.label("Users:"), styler.value(strings.Join(summary.Users, ", ")))
	}
}

func fetchRemotePublicIP(host string) (string, error) {
	ip, err := fetchRemoteIPFrom(host, "https://ipinfo.io/ip")
	if err == nil {
		return ip, nil
	}
	fallbackErr := err
	ip, err = fetchRemoteIPFrom(host, "https://api.ipify.org")
	if err == nil {
		return ip, nil
	}
	return "", fmt.Errorf("failed to fetch public IP: %v; fallback failed: %v", fallbackErr, err)
}

func runRemoteProxySetup(host string, domain string, publicIP string, username string, password string, proxyImage string) error {
	remoteArgs := []string{"/usr/local/bin/viberun-server", "proxy", "setup", "--domain", domain, "--public-ip", publicIP}
	if strings.TrimSpace(username) != "" {
		remoteArgs = append(remoteArgs, "--username", username)
	}
	if strings.TrimSpace(password) != "" {
		remoteArgs = append(remoteArgs, "--password-stdin")
	}
	if strings.TrimSpace(proxyImage) != "" {
		remoteArgs = append(remoteArgs, "--proxy-image", proxyImage)
	}
	remoteArgs = sshcmd.WithSudo(host, remoteArgs)
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	if strings.TrimSpace(password) != "" {
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "unknown flag: --domain") ||
			strings.Contains(lower, "unknown flag: --public-ip") ||
			strings.Contains(lower, "unknown flag: --username") ||
			strings.Contains(lower, "unknown flag: --password-stdin") ||
			strings.Contains(lower, "unknown flag: --proxy-image") {
			return fmt.Errorf("remote viberun-server is out of date; re-run bootstrap to update it (e.g. `viberun bootstrap --local %s`)", host)
		}
		if trimmed == "" {
			trimmed = err.Error()
		}
		return fmt.Errorf("%s", trimmed)
	}
	return nil
}

func fetchRemoteIPFrom(host string, url string) (string, error) {
	output, err := runRemoteCommand(host, []string{"curl", "-fsSL", url})
	if err != nil {
		return "", fmt.Errorf("curl %s failed: %v", url, err)
	}
	ip := strings.TrimSpace(output)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("unexpected public IP response: %q", ip)
	}
	return ip, nil
}

func runRemoteCommand(host string, remoteArgs []string) (string, error) {
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}

func fetchRemoteServerVersion(host string) (string, error) {
	remoteArgs := sshcmd.WithSudo(host, []string{"viberun-server", "version"})
	output, err := runRemoteCommandWithInput(host, remoteArgs, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func runRemoteCommandWithInput(host string, remoteArgs []string, input io.Reader) (string, error) {
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	if input != nil {
		cmd.Stdin = input
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return string(output), nil
}

type proxyInfo struct {
	App          string   `json:"app"`
	URL          string   `json:"url"`
	Access       string   `json:"access"`
	Disabled     bool     `json:"disabled"`
	CustomDomain string   `json:"custom_domain"`
	AllowedUsers []string `json:"allowed_users"`
	PrimaryUser  string   `json:"primary_user"`
	Users        []string `json:"users"`
	BaseDomain   string   `json:"base_domain"`
	PublicIP     string   `json:"public_ip"`
	Enabled      bool     `json:"enabled"`
}

type proxyConfigSummary struct {
	Enabled     bool     `json:"enabled"`
	BaseDomain  string   `json:"base_domain"`
	PublicIP    string   `json:"public_ip"`
	PrimaryUser string   `json:"primary_user"`
	ProxyImage  string   `json:"proxy_image"`
	Users       []string `json:"users"`
}

func fetchProxyInfo(resolved target.Resolved) (proxyInfo, error) {
	remoteArgs := []string{"viberun-server", "proxy", "info", resolved.App}
	remoteArgs = sshcmd.WithSudo(resolved.Host, remoteArgs)
	output, err := runRemoteCommandWithInput(resolved.Host, remoteArgs, nil)
	if err != nil {
		return proxyInfo{}, err
	}
	var info proxyInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return proxyInfo{}, fmt.Errorf("failed to parse proxy info: %w", err)
	}
	return info, nil
}

func fetchRemoteProxyConfig(host string) (proxyConfigSummary, error) {
	remoteArgs := sshcmd.WithSudo(host, []string{"viberun-server", "proxy", "config"})
	output, err := runRemoteCommandWithInput(host, remoteArgs, nil)
	if err != nil {
		return proxyConfigSummary{}, err
	}
	var summary proxyConfigSummary
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		return proxyConfigSummary{}, fmt.Errorf("failed to parse proxy config: %w", err)
	}
	return summary, nil
}

func runRemoteUsersList(host string) error {
	output, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, []string{"viberun-server", "proxy", "users", "list"}), nil)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, output)
	return nil
}

func runRemoteUsersAdd(host string, username string, password string) error {
	args := []string{"viberun-server", "proxy", "users", "add", "--username", username, "--password-stdin"}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), strings.NewReader(password+"\n"))
	return err
}

func runRemoteUsersRemove(host string, username string) error {
	args := []string{"viberun-server", "proxy", "users", "remove", "--username", username}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), nil)
	return err
}

func runRemoteUsersSetPassword(host string, username string, password string) error {
	args := []string{"viberun-server", "proxy", "users", "set-password", "--username", username, "--password-stdin"}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), strings.NewReader(password+"\n"))
	return err
}

func runRemoteWipe(host string) error {
	args := sshcmd.WithSudo(host, []string{"viberun-server", "wipe"})
	_, err := runRemoteCommandWithInput(host, args, nil)
	return err
}

func runRemoteAppsList(host string) ([]string, error) {
	output, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, []string{"viberun-server", "apps"}), nil)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	apps := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		apps = append(apps, name)
	}
	return apps, nil
}

func runRemoteAppUpdate(host string, app string) error {
	app = strings.TrimSpace(app)
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	if isDevRun() || isDevVersion() {
		if err := stageLocalImage(host); err != nil {
			return err
		}
	}
	remoteArgs := []string{"viberun-server", app, "update"}
	if isDevRun() || isDevVersion() {
		remoteArgs = append([]string{"env", "VIBERUN_SKIP_IMAGE_PULL=1"}, remoteArgs...)
	}
	remoteArgs = sshcmd.WithSudo(host, remoteArgs)
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, tty)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runRemoteSetAccess(host string, app string, access string) error {
	args := []string{"viberun-server", "proxy", "set-access", app, "--access", access}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), nil)
	return err
}

func runRemoteSetDomain(host string, app string, domain string, clear bool) error {
	args := []string{"viberun-server", "proxy", "set-domain", app}
	if clear {
		args = append(args, "--clear")
	} else {
		args = append(args, "--domain", domain)
	}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), nil)
	return err
}

func runRemoteSetDisabled(host string, app string, disabled bool) error {
	args := []string{"viberun-server", "proxy", "set-disabled", app}
	if disabled {
		args = append(args, "--disabled")
	} else {
		args = append(args, "--enabled")
	}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), nil)
	return err
}

func runRemoteSetUsers(host string, app string, users []string) error {
	joined := strings.Join(users, ",")
	args := []string{"viberun-server", "proxy", "set-users", app, "--users", joined}
	_, err := runRemoteCommandWithInput(host, sshcmd.WithSudo(host, args), nil)
	return err
}

type accessOverride struct {
	set    bool
	access string
}

func resolveAccessOverride(flags accessFlagValues) (accessOverride, error) {
	var override accessOverride
	if flags.makePublic.set {
		override = accessOverride{set: true, access: accessFromMakePublic(flags.makePublic.value)}
	}
	if flags.requireLogin.set {
		desired := accessFromRequireLogin(flags.requireLogin.value)
		if override.set && override.access != desired {
			return accessOverride{}, fmt.Errorf("choose either --make-public or --require-login")
		}
		override = accessOverride{set: true, access: desired}
	}
	return override, nil
}

func accessFromMakePublic(value bool) string {
	if value {
		return "public"
	}
	return "private"
}

func accessFromRequireLogin(value bool) string {
	if value {
		return "private"
	}
	return "public"
}

func applyURLUpdates(resolved target.Resolved, flags runFlags, access accessOverride) error {
	if flags.DisableURL && flags.EnableURL {
		return fmt.Errorf("choose either --disable or --enable")
	}
	if flags.SetDomain != "" && flags.ResetDomain {
		return fmt.Errorf("choose either --set-domain or --reset-domain")
	}
	if access.set {
		if err := runRemoteSetAccess(resolved.Host, resolved.App, access.access); err != nil {
			return err
		}
	}
	if flags.DisableURL {
		if err := runRemoteSetDisabled(resolved.Host, resolved.App, true); err != nil {
			return err
		}
	}
	if flags.EnableURL {
		if err := runRemoteSetDisabled(resolved.Host, resolved.App, false); err != nil {
			return err
		}
	}
	if flags.SetDomain != "" {
		if err := runRemoteSetDomain(resolved.Host, resolved.App, flags.SetDomain, false); err != nil {
			return err
		}
	}
	if flags.ResetDomain {
		if err := runRemoteSetDomain(resolved.Host, resolved.App, "", true); err != nil {
			return err
		}
	}
	return nil
}

func promptRecreateApps(host string) error {
	apps, err := runRemoteAppsList(host)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stdout, "App URLs changed. Re-run `viberun <app> update` to refresh environment variables.")
		return nil
	}
	prompt := fmt.Sprintf("Recreate %d app container(s) to set URL env vars? [Y/n]: ", len(apps))
	if !promptYesNoDefaultYes(prompt) {
		return nil
	}
	for _, app := range apps {
		fmt.Fprintf(os.Stdout, "Updating %s...\n", app)
		if err := runRemoteAppUpdate(host, app); err != nil {
			return err
		}
	}
	return nil
}

func maybeRecreateAppForURLChange(resolved target.Resolved, before proxyInfo, after proxyInfo) error {
	if before.URL == after.URL {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintf(os.Stdout, "App URL changed for %s. Run `viberun %s update` to refresh environment variables.\n", resolved.App, resolved.App)
		return nil
	}
	prompt := fmt.Sprintf("App URL changed for %s. Recreate container to update env vars? [Y/n]: ", resolved.App)
	if !promptYesNoDefaultYes(prompt) {
		return nil
	}
	return runRemoteAppUpdate(resolved.Host, resolved.App)
}

func printURLSummary(out io.Writer, info proxyInfo) {
	styler := newURLStyler(out)
	fmt.Fprintf(out, "%s %s\n", styler.label("App:"), styler.value(info.App))
	if info.Disabled {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("disabled (no URL)"))
	} else if info.Access == "public" {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("public"))
	} else {
		fmt.Fprintf(out, "%s %s\n", styler.label("Access:"), styler.status("requires login"))
	}
	if info.URL != "" {
		fmt.Fprintf(out, "%s %s\n", styler.label("URL:"), styler.link(info.URL))
	}
	if info.URL != "" && info.PublicIP != "" {
		host := strings.TrimPrefix(info.URL, "https://")
		host = strings.TrimPrefix(host, "http://")
		if host != "" {
			fmt.Fprintf(out, "%s %s\n", styler.label("DNS:"), styler.dnsLine(host, info.PublicIP))
		}
	}
	if info.CustomDomain != "" {
		fmt.Fprintf(out, "%s %s\n", styler.label("Custom domain:"), styler.value(info.CustomDomain))
	}
	if len(info.AllowedUsers) > 0 {
		fmt.Fprintf(out, "%s %s\n", styler.label("Users:"), styler.value(strings.Join(info.AllowedUsers, ", ")))
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, styler.header("Commands:"))
	styler.commands(out, []commandLine{
		{cmd: fmt.Sprintf("viberun %s url --set-domain <domain>", info.App), desc: "set a full domain (e.g., myblog.com)"},
		{cmd: fmt.Sprintf("viberun %s url --reset-domain", info.App), desc: "reset to the default domain"},
		{cmd: fmt.Sprintf("viberun %s users", info.App), desc: "manage who can access this app"},
		{cmd: fmt.Sprintf("viberun %s url --make-public", info.App), desc: "allow anyone to access"},
		{cmd: fmt.Sprintf("viberun %s url --require-login", info.App), desc: "require login to access"},
		{cmd: fmt.Sprintf("viberun %s url --disable", info.App), desc: "turn off the URL"},
		{cmd: fmt.Sprintf("viberun %s url --enable", info.App), desc: "turn the URL back on"},
	})
}

func printURLActionResult(out io.Writer, info proxyInfo) {
	styler := newURLStyler(out)
	status := "requires login"
	if info.Disabled {
		status = "disabled"
	} else if info.Access == "public" {
		status = "public"
	}
	if info.URL != "" && !info.Disabled {
		fmt.Fprintf(out, "%s %s %s\n", styler.value(info.App+":"), styler.status(status), styler.value(fmt.Sprintf("(%s)", info.URL)))
		return
	}
	fmt.Fprintf(out, "%s %s\n", styler.value(info.App+":"), styler.status(status))
}

type urlStyler struct {
	enabled       bool
	labelStyle    lipgloss.Style
	valueStyle    lipgloss.Style
	headerStyle   lipgloss.Style
	commandStyle  lipgloss.Style
	commentStyle  lipgloss.Style
	linkStyle     lipgloss.Style
	publicStyle   lipgloss.Style
	privateStyle  lipgloss.Style
	disabledStyle lipgloss.Style
	ipStyle       lipgloss.Style
}

type commandLine struct {
	cmd  string
	desc string
}

func newURLStyler(out io.Writer) urlStyler {
	theme := shellThemeForOutput(out)
	if !theme.Enabled {
		return urlStyler{}
	}
	return urlStyler{
		enabled:       true,
		labelStyle:    theme.Muted,
		valueStyle:    theme.Value,
		headerStyle:   theme.HelpHeader,
		commandStyle:  lipgloss.NewStyle(),
		commentStyle:  theme.Muted,
		linkStyle:     theme.Link,
		publicStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#1F7A1F", Dark: "#7EE787"}),
		privateStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#9A6B00", Dark: "#F2C14E"}),
		disabledStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#9CA3AF"}),
		ipStyle:       lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1F7A1F", Dark: "#7EE787"}),
	}
}

func (s urlStyler) label(text string) string {
	if !s.enabled {
		return text
	}
	return s.labelStyle.Render(text)
}

func (s urlStyler) value(text string) string {
	if !s.enabled {
		return text
	}
	return s.valueStyle.Render(text)
}

func (s urlStyler) link(text string) string {
	if !s.enabled {
		return text
	}
	return s.linkStyle.Render(text)
}

func (s urlStyler) ip(text string) string {
	if !s.enabled {
		return text
	}
	return s.ipStyle.Render(text)
}

func (s urlStyler) dnsLine(host string, ip string) string {
	if !s.enabled {
		return fmt.Sprintf("create an A record for %s -> %s", host, ip)
	}
	return fmt.Sprintf("create an A record for %s -> %s", s.linkStyle.Render(host), s.ipStyle.Render(ip))
}

func (s urlStyler) header(text string) string {
	if !s.enabled {
		return text
	}
	return s.headerStyle.Render(text)
}

func (s urlStyler) status(text string) string {
	if !s.enabled {
		return text
	}
	switch text {
	case "public":
		return s.publicStyle.Render(text)
	case "private", "requires login":
		return s.privateStyle.Render(text)
	case "disabled", "disabled (no URL)":
		return s.disabledStyle.Render(text)
	default:
		return s.valueStyle.Render(text)
	}
}

func (s urlStyler) commands(out io.Writer, lines []commandLine) {
	if len(lines) == 0 {
		return
	}
	maxWidth := 0
	for _, line := range lines {
		width := utf8.RuneCountInString(line.cmd)
		if width > maxWidth {
			maxWidth = width
		}
	}
	for _, line := range lines {
		padding := maxWidth - utf8.RuneCountInString(line.cmd)
		if padding < 0 {
			padding = 0
		}
		cmd := line.cmd + strings.Repeat(" ", padding)
		comment := "# " + line.desc
		if s.enabled {
			fmt.Fprintf(out, "  %s  %s\n", s.commandStyle.Render(cmd), s.commentStyle.Render(comment))
			continue
		}
		fmt.Fprintf(out, "  %s  %s\n", cmd, comment)
	}
}

type loadOutputStyler struct {
	out    io.Writer
	styler urlStyler
	buffer []byte
}

func newLoadOutputStyler(out io.Writer) *loadOutputStyler {
	return &loadOutputStyler{
		out:    out,
		styler: newURLStyler(out),
	}
}

func (s *loadOutputStyler) Write(p []byte) (int, error) {
	s.buffer = append(s.buffer, p...)
	for {
		idx := bytes.IndexByte(s.buffer, '\n')
		if idx == -1 {
			break
		}
		line := string(s.buffer[:idx])
		s.buffer = s.buffer[idx+1:]
		if err := s.writeLine(line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (s *loadOutputStyler) Flush() error {
	if len(s.buffer) == 0 {
		return nil
	}
	line := string(s.buffer)
	s.buffer = s.buffer[:0]
	return s.writeLine(line)
}

func (s *loadOutputStyler) writeLine(line string) error {
	line = strings.TrimSuffix(line, "\r")
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "Loaded image:") {
		tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "Loaded image:"))
		if tag != "" {
			_, err := fmt.Fprintf(s.out, "%s %s\n", s.styler.label("Loaded image:"), s.styler.value(tag))
			return err
		}
	}
	_, err := fmt.Fprintln(s.out, line)
	return err
}

func wantPrettyOutput(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	termValue := os.Getenv("TERM")
	if termValue == "" || termValue == "dumb" {
		return false
	}
	if ttyAware, ok := out.(interface{ IsTTY() bool }); ok {
		return ttyAware.IsTTY()
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func runUsersEditor(resolved target.Resolved, info proxyInfo) (bool, error) {
	primary := strings.TrimSpace(info.PrimaryUser)
	secondaryOptions := make([]huh.Option[string], 0, len(info.Users))
	selected := make([]string, 0, len(info.AllowedUsers))
	allowed := map[string]bool{}
	for _, user := range info.AllowedUsers {
		allowed[user] = true
	}
	for _, user := range info.Users {
		if strings.EqualFold(user, primary) {
			continue
		}
		option := huh.NewOption(user, user)
		if allowed[user] {
			option = option.Selected(true)
			selected = append(selected, user)
		}
		secondaryOptions = append(secondaryOptions, option)
	}
	if len(secondaryOptions) == 0 {
		fmt.Fprintln(os.Stdout, "No secondary users configured. Add one with: viberun users add --username <u>")
		return false, nil
	}
	title := "Users with access"
	desc := "Primary user " + primary + " always has access. Clear all to reset to primary only."
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description(desc).
				Options(secondaryOptions...).
				Value(&selected),
		),
	)
	form.WithInput(os.Stdin).WithOutput(os.Stdout).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return false, err
	}
	selected = proxy.NormalizeUserList(selected)
	if sameStringSet(selected, filterSecondary(info.AllowedUsers, primary)) {
		return false, nil
	}
	if err := runRemoteSetUsers(resolved.Host, resolved.App, selected); err != nil {
		return false, err
	}
	return true, nil
}

func filterSecondary(users []string, primary string) []string {
	out := make([]string, 0, len(users))
	for _, user := range users {
		if strings.EqualFold(user, primary) {
			continue
		}
		out = append(out, user)
	}
	return proxy.NormalizeUserList(out)
}

func sameStringSet(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, val := range a {
		seen[val]++
	}
	for _, val := range b {
		if seen[val] == 0 {
			return false
		}
		seen[val]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func configFlagsEmpty(flags configFlags) bool {
	return strings.TrimSpace(flags.Host) == "" &&
		strings.TrimSpace(flags.DefaultHost) == "" &&
		strings.TrimSpace(flags.Agent) == "" &&
		len(flags.SetHosts) == 0
}

func runApp(flags runFlags, args runArgs, accessFlags accessFlagValues) error {
	targetArg := strings.TrimSpace(args.Target)
	if targetArg == "" {
		return exitUsage(runUsageFull())
	}

	actionArgs := []string{}
	action := strings.TrimSpace(args.Action)
	value := strings.TrimSpace(args.Value)
	wantURL := false
	wantUsers := false
	if action != "" {
		switch action {
		case "snapshot":
			if value != "" {
				return exitUsage(runUsageActions())
			}
			actionArgs = []string{"snapshot"}
		case "snapshots":
			if value != "" {
				return exitUsage(runUsageActions())
			}
			actionArgs = []string{"snapshots"}
		case "shell":
			if value != "" {
				return exitUsage(runUsageActions())
			}
			actionArgs = []string{"shell"}
		case "url":
			if value != "" {
				return exitUsage("Usage: viberun <app> url [flags]")
			}
			wantURL = true
		case "users":
			if value != "" {
				return exitUsage("Usage: viberun <app> users")
			}
			wantUsers = true
		case "restore":
			if value == "" {
				return exitUsage(runUsageRestore())
			}
			actionArgs = []string{"restore", value}
		default:
			return exitUsage(runUsageActions())
		}
	}
	if flags.Delete {
		if len(actionArgs) != 0 {
			return exitUsage(runUsageDelete())
		}
		if !flags.Yes {
			if !promptDelete(targetArg) {
				fmt.Fprintln(os.Stdout, "delete cancelled")
				return nil
			}
		}
		actionArgs = []string{"delete"}
	}

	if flags.Open && !wantURL {
		return exitUsage("Usage: viberun <app> url [flags]")
	}
	accessOverride, err := resolveAccessOverride(accessFlags)
	if err != nil {
		return exitUsage(err.Error())
	}
	if (accessOverride.set || flags.DisableURL || flags.EnableURL || flags.SetDomain != "" || flags.ResetDomain) && !wantURL {
		return exitUsage("Usage: viberun <app> url [--make-public|--require-login|--disable|--enable|--set-domain <domain>|--reset-domain]")
	}
	if wantUsers && !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("app users requires a TTY")
	}
	interactive := !wantURL && !wantUsers && (len(actionArgs) == 0 || (len(actionArgs) == 1 && actionArgs[0] == "shell"))
	tty := interactive && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	explicitAgent := strings.TrimSpace(flags.Agent)
	configuredAgent := strings.TrimSpace(cfg.AgentProvider)
	agentProvider := explicitAgent
	if agentProvider == "" {
		agentProvider = configuredAgent
	}
	needsAgentPrompt := !wantURL && !wantUsers && agentProvider == "" && tty
	agentProviderForChecks := agentProvider
	if agentProviderForChecks == "" {
		agentProviderForChecks = agents.DefaultProvider()
	}

	resolved, err := target.Resolve(targetArg, cfg)
	if err != nil {
		if errors.Is(err, target.ErrNoHostConfigured) {
			return missingHostError{}
		}
		return exitUsage(fmt.Sprintf("invalid target: %v", err))
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return fmt.Errorf("ssh is required but was not found in PATH")
	}
	if err := ensureDevServerSynced(resolved.Host); err != nil {
		return err
	}

	if wantURL {
		changesRequested := accessOverride.set || flags.DisableURL || flags.EnableURL || flags.SetDomain != "" || flags.ResetDomain
		var before proxyInfo
		if changesRequested {
			info, err := fetchProxyInfo(resolved)
			if err != nil {
				return err
			}
			before = info
			if err := applyURLUpdates(resolved, flags, accessOverride); err != nil {
				return err
			}
		}
		info, err := fetchProxyInfo(resolved)
		if err != nil {
			return err
		}
		if changesRequested {
			printURLActionResult(os.Stdout, info)
		} else {
			printURLSummary(os.Stdout, info)
		}
		if flags.Open {
			if info.Disabled || strings.TrimSpace(info.URL) == "" {
				return fmt.Errorf("URL is not available")
			}
			if err := openURL(info.URL); err != nil {
				return err
			}
		}
		if changesRequested {
			if err := maybeRecreateAppForURLChange(resolved, before, info); err != nil {
				return err
			}
		}
		return nil
	}

	if wantUsers {
		info, err := fetchProxyInfo(resolved)
		if err != nil {
			return err
		}
		updated, err := runUsersEditor(resolved, info)
		if err != nil {
			return err
		}
		if updated {
			fmt.Fprintln(os.Stdout, "Updated app user access.")
		}
		return nil
	}

	if interactive && !tty {
		return fmt.Errorf("interactive sessions require a TTY; run from a terminal or use snapshot/restore commands")
	}
	extraEnv := map[string]string{}
	if tty {
		if colorTerm := strings.TrimSpace(os.Getenv("COLORTERM")); colorTerm != "" {
			extraEnv["COLORTERM"] = colorTerm
		}
	}
	if flags.ForwardAgent {
		extraEnv["VIBERUN_FORWARD_AGENT"] = "1"
	}
	needsCreate := false
	if interactive && tty {
		exists, err := remoteContainerExists(resolved, agentProviderForChecks)
		if err != nil {
			return err
		}
		if !exists {
			if !promptCreateLocal(resolved.App) {
				fmt.Fprintln(os.Stderr, "aborted")
				return newSilentError(errors.New("aborted"))
			}
			needsCreate = true
			extraEnv["VIBERUN_AUTO_CREATE"] = "1"
		}
	}
	if needsAgentPrompt {
		selection, err := tui.SelectDefaultAgent(os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if strings.TrimSpace(selection) != "" {
			cfg.AgentProvider = selection
			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			agentProvider = selection
		}
	}
	if strings.TrimSpace(agentProvider) == "" {
		agentProvider = agentProviderForChecks
	}
	agentSpec, err := agents.Resolve(agentProvider)
	if err != nil {
		return err
	}
	agentProvider = agentSpec.Provider
	if interactive && tty && needsCreate {
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
	if interactive {
		cfg, err := discoverLocalUserConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "user config discovery failed: %v\n", err)
		} else if encoded, err := encodeUserConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode user config: %v\n", err)
		} else if encoded != "" {
			extraEnv["VIBERUN_USER_CONFIG"] = encoded
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
	remoteArgs = sshcmd.WithSudo(resolved.Host, remoteArgs)
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
	if flags.ForwardAgent {
		sshArgs = append([]string{"-A"}, sshArgs...)
	}
	var outputTail tailBuffer
	outputTail.max = 32 * 1024
	if interactive && tty {
		if err := runInteractiveSSHProxy(resolved, sshArgs, &outputTail); err != nil {
			cleanup()
			if exitErr, ok := err.(*exec.ExitError); ok {
				maybeClearDefaultAgentOnFailure(cfg, cfgPath, flags, resolved.App, outputTail.String())
				return newSilentError(exitErr)
			}
			return fmt.Errorf("failed to start ssh: %w", err)
		}
		return nil
	}
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputTail)
	cmd.Stderr = io.MultiWriter(os.Stderr, &outputTail)

	if err := cmd.Run(); err != nil {
		cleanup()
		if exitErr, ok := err.(*exec.ExitError); ok {
			maybeClearDefaultAgentOnFailure(cfg, cfgPath, flags, resolved.App, outputTail.String())
			return newSilentError(exitErr)
		}
		return fmt.Errorf("failed to start ssh: %w", err)
	}
	return nil
}

func exitUsage(message string) error {
	return newUsageError(message)
}

func runUsageFull() string {
	return "Usage: viberun [--agent provider] [--forward-agent|-A] <app> | viberun [--agent provider] [--forward-agent|-A] <app>@<host> | viberun [--agent provider] [--forward-agent|-A] <app> snapshot | viberun [--agent provider] [--forward-agent|-A] <app> snapshots | viberun [--agent provider] [--forward-agent|-A] <app> restore <snapshot> | viberun <app> shell | viberun <app> url [flags] | viberun <app> users | viberun <app> --delete [-y] | viberun bootstrap [<host>] | viberun proxy setup [<host>] | viberun users <list|add|remove|set-password> | viberun wipe [<host>] | viberun config [options]"
}

func runUsageActions() string {
	return "Usage: viberun [--agent provider] [--forward-agent|-A] <app> snapshot | viberun [--agent provider] [--forward-agent|-A] <app> snapshots | viberun [--agent provider] [--forward-agent|-A] <app> restore <snapshot> | viberun <app> shell | viberun <app> url [flags] | viberun <app> users"
}

func runUsageRestore() string {
	return "Usage: viberun [--agent provider] [--forward-agent|-A] <app> restore <snapshot>"
}

func runUsageDelete() string {
	return "Usage: viberun [--delete] <app> | viberun [--agent provider] [--forward-agent|-A] <app> snapshot | viberun [--agent provider] [--forward-agent|-A] <app> snapshots | viberun [--agent provider] [--forward-agent|-A] <app> restore <snapshot> | viberun <app> shell | viberun <app> url [flags]"
}

func printMissingHostMessage() {
	fmt.Fprintln(os.Stderr, "No host is configured yet, so viberun does not know where to run your app.")
	fmt.Fprintln(os.Stderr, "Please run bootstrap against a host first:")
	fmt.Fprintln(os.Stderr, "  viberun bootstrap <host>   (example: viberun bootstrap user@your-host)")
	fmt.Fprintln(os.Stderr, "Then retry with:")
	fmt.Fprintln(os.Stderr, "  viberun <app>")
	fmt.Fprintln(os.Stderr, "Or run once with an explicit host:")
	fmt.Fprintln(os.Stderr, "  viberun <app>@<host>")
	fmt.Fprintln(os.Stderr, "To set a default host for future runs:")
	fmt.Fprintln(os.Stderr, "  viberun config --host <host>")
}

func maybeClearDefaultAgentOnFailure(cfg config.Config, cfgPath string, flags runFlags, app string, stderrOutput string) {
	if strings.TrimSpace(flags.Agent) != "" {
		return
	}
	runner, pkg, ok := customAgentParts(cfg.AgentProvider)
	if !ok || strings.TrimSpace(pkg) == "" {
		return
	}
	if !looksLikeCustomAgentFailure(stderrOutput) {
		return
	}
	cfg.AgentProvider = ""
	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to clear default agent in %s: %v\n", cfgPath, err)
		return
	}
	fmt.Fprintf(os.Stderr, "cleared default agent in %s so the next run prompts you to choose an agent\n", cfgPath)
	if strings.TrimSpace(app) != "" {
		fmt.Fprintf(os.Stderr, "test the agent locally: viberun %s shell, then run: %s %s --help\n", app, runner, pkg)
	}
	fmt.Fprintf(os.Stderr, "then retry with --agent %s:%s or set it as default with: viberun config --agent %s:%s\n", runner, pkg, runner, pkg)
}

func versionString() string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		trimmed = "dev"
	}
	extra := []string{}
	if strings.TrimSpace(commit) != "" {
		extra = append(extra, strings.TrimSpace(commit))
	}
	if len(extra) == 0 {
		return trimmed
	}
	return fmt.Sprintf("%s (%s)", trimmed, strings.Join(extra, " "))
}

type semver struct {
	major int
	minor int
	patch int
}

func compareSemver(left string, right string) (int, bool) {
	leftVer, ok := parseSemver(left)
	if !ok {
		return 0, false
	}
	rightVer, ok := parseSemver(right)
	if !ok {
		return 0, false
	}
	if leftVer.major != rightVer.major {
		if leftVer.major < rightVer.major {
			return -1, true
		}
		return 1, true
	}
	if leftVer.minor != rightVer.minor {
		if leftVer.minor < rightVer.minor {
			return -1, true
		}
		return 1, true
	}
	if leftVer.patch != rightVer.patch {
		if leftVer.patch < rightVer.patch {
			return -1, true
		}
		return 1, true
	}
	return 0, true
}

func parseSemver(value string) (semver, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return semver{}, false
	}
	trimmed = strings.TrimPrefix(trimmed, "v")
	trimmed = strings.TrimPrefix(trimmed, "V")
	if idx := strings.IndexAny(trimmed, "-+"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 3 {
		return semver{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}
	return semver{major: major, minor: minor, patch: patch}, true
}

func runInteractiveSSHProxy(resolved target.Resolved, sshArgs []string, outputTail *tailBuffer) error {
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() {
		_ = ptmx.Close()
	}()

	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(fd, state)
	}()

	_ = pty.InheritSize(os.Stdin, ptmx)
	stopResize := startResizeWatcher(ptmx, os.Stdin)

	copyDone := make(chan struct{})
	go func() {
		if outputTail != nil {
			_, _ = io.Copy(io.MultiWriter(os.Stdout, outputTail), ptmx)
		} else {
			_, _ = io.Copy(os.Stdout, ptmx)
		}
		close(copyDone)
	}()

	go func() {
		buf := make([]byte, 256)
		for {
			n, readErr := os.Stdin.Read(buf)
			writeFailed := false
			if n > 0 {
				for i := 0; i < n; i++ {
					if buf[i] == 0x16 {
						handleClipboardImagePaste(resolved, ptmx)
						continue
					}
					if _, err := ptmx.Write(buf[i : i+1]); err != nil {
						writeFailed = true
						break
					}
				}
			}
			if readErr != nil || writeFailed {
				break
			}
		}
	}()

	err = cmd.Wait()
	if stopResize != nil {
		stopResize()
	}
	<-copyDone
	return err
}

func handleClipboardImagePaste(resolved target.Resolved, ptmx *os.File) {
	png, err := clipboard.ReadImagePNG()
	if err != nil {
		return
	}
	path, err := uploadClipboardImage(resolved, png)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipboard image upload failed: %v\n", err)
		return
	}
	_, _ = ptmx.Write([]byte(path + " "))
}

func uploadClipboardImage(resolved target.Resolved, png []byte) (string, error) {
	path, err := newClipboardImagePath()
	if err != nil {
		return "", err
	}
	container := fmt.Sprintf("viberun-%s", resolved.App)
	writeArg := "cat > " + shellQuote(path)
	if !isLocalHost(resolved.Host) {
		writeArg = shellQuote(writeArg)
	}
	writeCmd := []string{"docker", "exec", "-i", container, "sh", "-c", writeArg}
	var cmd *exec.Cmd
	if isLocalHost(resolved.Host) {
		cmd = exec.Command(writeCmd[0], writeCmd[1:]...)
	} else {
		sshArgs := sshcmd.BuildArgs(resolved.Host, writeCmd, false)
		cmd = exec.Command("ssh", sshArgs...)
		cmd.Env = normalizedSshEnv()
	}
	cmd.Stdin = bytes.NewReader(png)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return path, nil
}

func newClipboardImagePath() (string, error) {
	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("/tmp/viberun-clip-%d-%s.png", time.Now().UnixNano(), hex.EncodeToString(randBytes)), nil
}

func looksLikeCustomAgentFailure(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(normalized, "custom agent") && strings.Contains(normalized, "failed to start inside the container")
}

func customAgentParts(provider string) (string, string, bool) {
	normalized := strings.TrimSpace(provider)
	lowered := strings.ToLower(normalized)
	switch {
	case strings.HasPrefix(lowered, "npx:"):
		pkg := strings.TrimSpace(normalized[len("npx:"):])
		return "npx", pkg, true
	case strings.HasPrefix(lowered, "uvx:"):
		pkg := strings.TrimSpace(normalized[len("uvx:"):])
		return "uvx", pkg, true
	default:
		return "", "", false
	}
}

type tailBuffer struct {
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	if t.max <= 0 || len(p) == 0 {
		return len(p), nil
	}
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		return len(p), nil
	}
	if len(t.buf)+len(p) > t.max {
		cut := len(t.buf) + len(p) - t.max
		if cut < len(t.buf) {
			t.buf = append(t.buf[cut:], p...)
		} else {
			t.buf = append(t.buf[:0], p...)
		}
		return len(p), nil
	}
	t.buf = append(t.buf, p...)
	return len(p), nil
}

func (t *tailBuffer) String() string {
	return string(t.buf)
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

VIBERUN_SERVER_REPO="${VIBERUN_SERVER_REPO:-shayne/viberun}"
VIBERUN_SERVER_VERSION="${VIBERUN_SERVER_VERSION:-latest}"
VIBERUN_SERVER_INSTALL_DIR="${VIBERUN_SERVER_INSTALL_DIR:-/usr/local/bin}"
VIBERUN_SERVER_BIN="${VIBERUN_SERVER_BIN:-viberun-server}"
VIBERUN_IMAGE="${VIBERUN_IMAGE:-}"
VIBERUN_PROXY_IMAGE="${VIBERUN_PROXY_IMAGE:-ghcr.io/shayne/viberun/viberun-proxy:latest}"
VIBERUN_SERVER_PATH="${VIBERUN_SERVER_INSTALL_DIR}/${VIBERUN_SERVER_BIN}"
VIBERUN_SUDOERS_FILE="/etc/sudoers.d/viberun-server"

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
  if [ ! -f "$VIBERUN_SUDOERS_FILE" ]; then
    echo "viberun needs passwordless sudo and full environment access for viberun-server." >&2
    printf "Allow viberun to add a sudoers entry for %s? [y/N]: " "$VIBERUN_SERVER_PATH" >&2
    read -r reply
    case "$reply" in
      y|Y|yes|YES)
        $SUDO sh -c "cat > \"$VIBERUN_SUDOERS_FILE\" <<EOF\nDefaults!$VIBERUN_SERVER_PATH !env_reset\n$USER ALL=(root) NOPASSWD: $VIBERUN_SERVER_PATH\nEOF"
        $SUDO chmod 0440 "$VIBERUN_SUDOERS_FILE"
        ;;
      *)
        echo "cannot continue without passwordless sudo for viberun-server" >&2
        exit 1
        ;;
    esac
  fi
fi

	if ! need_cmd curl; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y curl ca-certificates
fi

if ! need_cmd ss; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y iproute2
fi

if ! need_cmd netstat; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y net-tools
fi

if ! need_cmd mkfs.btrfs || ! need_cmd btrfs; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y btrfs-progs
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
  if [ -t 1 ]; then
    tmpfile="$(mktemp)"
    spin=( '⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏' )
    set +e
    ($SUDO docker pull --quiet "$image" >"$tmpfile" 2>&1) &
    pid=$!
    idx=0
    while kill -0 "$pid" 2>/dev/null; do
      printf "\r%s Pulling %s" "${spin[$idx]}" "$image"
      idx=$(( (idx + 1) % ${#spin[@]} ))
      sleep 0.12
    done
    wait "$pid"
    status=$?
    set -e
    if [ "$status" -ne 0 ]; then
      output="$(cat "$tmpfile")"
      printf "\r✖ Pulling %s failed\n" "$image"
      echo "warning: failed to pull image $image: $output" >&2
      rm -f "$tmpfile"
      return 0
    fi
    printf "\r✔ Pulled %s\n" "$image"
    rm -f "$tmpfile"
    return 0
  fi
  if ! output="$($SUDO docker pull --quiet "$image" 2>&1)"; then
    echo "warning: failed to pull image $image: $output" >&2
  fi
  return 0
}

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
  if [ -z "${VIBERUN_SKIP_IMAGE_PULL:-}" ] && [ -n "$VIBERUN_IMAGE" ]; then
    $SUDO docker tag "$VIBERUN_IMAGE" "viberun:latest" || true
  fi
  pull_image "$VIBERUN_PROXY_IMAGE"
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

if [ -n "${VIBERUN_SKIP_SERVER_INSTALL:-}" ]; then
  if [ ! -x "$VIBERUN_SERVER_PATH" ]; then
    echo "missing viberun-server at $VIBERUN_SERVER_PATH; cannot skip install" >&2
    exit 1
  fi
else
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

  $SUDO install -m 0755 "$binary_path" "$VIBERUN_SERVER_PATH"
  if [ "$VIBERUN_SERVER_PATH" != "/usr/local/bin/viberun-server" ]; then
    $SUDO ln -sf "$VIBERUN_SERVER_PATH" "/usr/local/bin/viberun-server"
  fi
fi
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
	remoteArgs = sshcmd.WithSudo(resolved.Host, remoteArgs)
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
	remoteArgs = sshcmd.WithSudo(resolved.Host, remoteArgs)
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

func promptYesNoDefaultYes(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, prompt)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "" || input == "y" || input == "yes"
}

func promptYesNoDefaultNo(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, prompt)
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

func isDevVersion() bool {
	trimmed := strings.TrimSpace(version)
	return trimmed == "" || trimmed == "dev"
}

func ensureDevServerSynced(host string) error {
	if !isDevRun() && !isDevVersion() {
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	devServerSync.mu.Lock()
	if devServerSync.hosts == nil {
		devServerSync.hosts = map[string]bool{}
	}
	if devServerSync.hosts[host] {
		devServerSync.mu.Unlock()
		return nil
	}
	devServerSync.mu.Unlock()

	remotePath, err := stageLocalServerBinary(host, "")
	if err != nil {
		return err
	}
	if err := installRemoteServerBinary(host, remotePath); err != nil {
		return err
	}
	devServerSync.mu.Lock()
	devServerSync.hosts[host] = true
	devServerSync.mu.Unlock()
	return nil
}

func installRemoteServerBinary(host string, remotePath string) error {
	target := "/usr/local/bin/viberun-server"
	remoteArgs := []string{"install", "-m", "0755", remotePath, target}
	if hostNeedsSudo(host) {
		remoteArgs = append([]string{"sudo", "-n"}, remoteArgs...)
	}
	sshArgs := sshcmd.BuildArgs(host, remoteArgs, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = err.Error()
		}
		return fmt.Errorf("failed to install viberun-server: %s", trimmed)
	}
	cleanupArgs := []string{"rm", "-f", remotePath}
	if hostNeedsSudo(host) {
		cleanupArgs = append([]string{"sudo", "-n"}, cleanupArgs...)
	}
	cleanupSSH := sshcmd.BuildArgs(host, cleanupArgs, false)
	cleanupSSH = append([]string{"-o", "LogLevel=ERROR"}, cleanupSSH...)
	cleanupCmd := exec.Command("ssh", cleanupSSH...)
	cleanupCmd.Env = normalizedSshEnv()
	_ = cleanupCmd.Run()
	return nil
}

func hostNeedsSudo(host string) bool {
	user := parseHostUser(host)
	return user != "" && user != "root"
}

func parseHostUser(host string) string {
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

func stageLocalImage(host string) error {
	if err := stageLocalDockerImage(host, "viberun:dev", "Dockerfile"); err != nil {
		return err
	}
	return tagRemoteImage(host, "viberun:dev", "viberun:latest")
}

func stageLocalProxyImage(host string) error {
	return stageLocalDockerImage(host, devProxyImageTag(), "Dockerfile.proxy")
}

func devProxyImageTag() string {
	return "viberun-proxy:dev"
}

func stageLocalDockerImage(host string, tag string, dockerfile string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required to build the image locally")
	}
	_, arch, err := detectRemotePlatform(host)
	if err != nil {
		return err
	}
	if strings.TrimSpace(dockerfile) == "" {
		dockerfile = "Dockerfile"
	}
	buildCmd := exec.Command("docker", "build", "--platform", "linux/"+arch, "-t", tag, "-f", dockerfile, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(tag) != "" {
		localID, err := localDockerImageID(tag)
		if err == nil && localID != "" {
			if remoteID, err := remoteDockerImageID(host, tag); err == nil && remoteID == localID {
				fmt.Fprintf(os.Stdout, "Remote image %s is already up to date; skipping upload.\n", tag)
				return nil
			}
		}
	}
	sshArgs := sshcmd.BuildArgs(host, []string{"docker", "load"}, false)
	loadCmd := exec.Command("ssh", sshArgs...)
	loadCmd.Env = normalizedSshEnv()
	loadOutput := newLoadOutputStyler(os.Stdout)
	loadCmd.Stdout = loadOutput
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
		_ = loadOutput.Flush()
		return err
	}
	if err := loadOutput.Flush(); err != nil {
		return err
	}
	return nil
}

func localDockerImageID(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", nil
	}
	cmd := exec.Command("docker", "image", "inspect", "-f", "{{.Id}}", tag)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func remoteDockerImageID(host string, tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", nil
	}
	sshArgs := sshcmd.BuildArgs(host, []string{"docker", "image", "inspect", "-f", "{{.Id}}", tag}, false)
	sshArgs = append([]string{"-o", "LogLevel=ERROR"}, sshArgs...)
	cmd := exec.Command("ssh", sshArgs...)
	cmd.Env = normalizedSshEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

func tagRemoteImage(host string, source string, target string) error {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return nil
	}
	tagArgs := sshcmd.BuildArgs(host, []string{"docker", "tag", source, target}, false)
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
