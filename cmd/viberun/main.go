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
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/shayne/viberun/internal/config"
	"github.com/shayne/viberun/internal/proxy"
	"github.com/shayne/viberun/internal/sshcmd"
	"github.com/shayne/viberun/internal/target"
	"github.com/shayne/viberun/internal/tui"
	"github.com/shayne/viberun/internal/tui/theme"
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
	skip  map[string]bool
}{}

func runCLI() error {
	if shouldStartShell() {
		return runShell()
	}
	args := normalizeArgs(os.Args[1:])
	if len(args) == 0 {
		args = []string{"--help"}
	}
	if len(args) > 0 && args[0] == "attach" {
		return handleAttach(args[1:])
	}
	if hasVersionFlag(args) {
		fmt.Fprintln(os.Stdout, versionString())
		return nil
	}
	handlers := map[string]yargs.SubcommandHandler{
		"attach": handleAttachCommand,
		"setup":  handleSetupCommand,
		"wipe":   handleWipeCommand,
	}
	if err := yargs.RunSubcommands(context.Background(), args, helpConfig, struct{}{}, handlers); err != nil {
		if errors.Is(err, yargs.ErrShown) {
			return nil
		}
		return err
	}
	return nil
}

type setupArgs struct {
	Host string `pos:"0?" help:"host to set up"`
}

type wipeArgs struct {
	Host string `pos:"0?" help:"host to wipe"`
}

type attachFlags struct {
	Host  string `flag:"host" help:"host override"`
	Agent string `flag:"agent" help:"agent override"`
	Shell bool   `flag:"shell" help:"open an app shell instead of the agent session"`
}

type attachArgs struct {
	App string `pos:"0" help:"app to attach"`
}

var helpConfig = yargs.HelpConfig{
	Command: yargs.CommandInfo{
		Name:        "viberun",
		Description: "Shell-first agent app host",
		Examples: []string{
			"viberun",
			"viberun --help",
			"viberun attach myapp",
			"viberun help setup",
			"viberun --version",
			"viberun setup",
			"viberun wipe",
		},
	},
	SubCommands: map[string]yargs.SubCommandInfo{
		"attach": {
			Name:        "attach",
			Description: "Attach to an app session (internal)",
			Usage:       "[--host <host>] [--agent <provider>] [--shell] <app>",
		},
		"setup": {
			Name:        "setup",
			Description: "Connect a server and install viberun",
			Usage:       "[<host>]",
		},
		"wipe": {
			Name:        "wipe",
			Description: "Wipe local config and all viberun state on a host",
			Usage:       "[<host>]",
		},
	},
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if args[0] == "version" {
		return []string{"--version"}
	}
	if args[0] == "bootstrap" {
		args = append([]string{"setup"}, args[1:]...)
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
	if args[0] == "bootstrap" {
		return []string{"setup", helpFlag}
	}
	return []string{helpFlag}
}

func isKnownCommand(value string) bool {
	switch value {
	case "setup", "wipe":
		return true
	default:
		return false
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

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "--version", "-v":
			return true
		}
	}
	return false
}

func handleSetupCommand(_ context.Context, args []string) error {
	return handleSetup(args)
}
func handleSetup(args []string) error {
	result, err := yargs.ParseAndHandleHelp[struct{}, struct{}, setupArgs](args, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	state, err := newShellState()
	if err != nil {
		return err
	}
	state.bootstrapped = strings.TrimSpace(state.host) != ""
	state.setupNeeded = false
	action := setupAction{host: strings.TrimSpace(result.Args.Host)}
	ran, note, err := runShellSetup(state, action)
	if note != "" {
		fmt.Fprintln(os.Stdout, note)
	}
	if err != nil {
		return err
	}
	if !ran {
		return nil
	}
	return nil
}

func handleWipeCommand(_ context.Context, args []string) error {
	return handleWipe(args)
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

func handleAttachCommand(_ context.Context, args []string) error {
	return handleAttach(args)
}

func handleAttach(args []string) error {
	parseArgs := append([]string{"attach"}, args...)
	result, err := yargs.ParseAndHandleHelp[struct{}, attachFlags, attachArgs](parseArgs, helpConfig)
	if errors.Is(err, yargs.ErrShown) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Args.App) == "" {
		return newUsageError("attach requires an app name")
	}
	state, err := newShellState()
	if err != nil {
		return err
	}
	if host := strings.TrimSpace(result.SubCommandFlags.Host); host != "" {
		state.host = host
	}
	if agent := strings.TrimSpace(result.SubCommandFlags.Agent); agent != "" {
		state.agent = agent
	}
	action := ""
	if result.SubCommandFlags.Shell {
		action = "shell"
	}
	return runShellInteractive(state, result.Args.App, action)
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
	proxyImage      string
}

func runProxySetupFlow(host string, gateway *gatewayClient, opts proxySetupOptions) error {
	configSummary, err := fetchRemoteProxyConfig(gateway)
	configured := err == nil && configSummary.Enabled && strings.TrimSpace(configSummary.BaseDomain) != "" && strings.TrimSpace(configSummary.PrimaryUser) != ""
	if err != nil && !opts.forceSetup {
		if !promptYesNoDefaultNo("Existing public domain settings could not be read. Continue with setup? [y/N]: ") {
			if opts.showSkipHint {
				fmt.Fprintln(os.Stdout, "Set up a public domain name later with: proxy setup")
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
				publicIP, err = fetchRemotePublicIP(gateway)
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
			publicIP, err = fetchRemotePublicIP(gateway)
			if err != nil {
				return err
			}
		}
	} else {
		if !opts.forceSetup && !promptProxySetup() {
			if opts.showSkipHint {
				fmt.Fprintln(os.Stdout, "Set up a public domain name later with: proxy setup")
			}
			return nil
		}
		shouldRunSetup = true
		publicIP, err = fetchRemotePublicIP(gateway)
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

	proxyImage := strings.TrimSpace(opts.proxyImage)
	if shouldRunSetup && opts.updateArtifacts && (isDevRun() || isDevVersion()) {
		if err := ensureDevServerSynced(host); err != nil {
			return err
		}
		if proxyImage == "" {
			if err := stageLocalProxyImage(host); err != nil {
				return err
			}
			proxyImage = devProxyImageTag()
		}
	}

	domainChanged := !configured || strings.TrimSpace(domain) != strings.TrimSpace(configSummary.BaseDomain)
	if err := runRemoteProxySetup(gateway, domain, publicIP, username, password, proxyImage); err != nil {
		return err
	}
	if !configured || editConfig {
		printProxySetupInstructions(domain, publicIP, username)
	}
	if domainChanged {
		if err := promptRecreateApps(gateway, host); err != nil {
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

func fetchRemotePublicIP(gateway *gatewayClient) (string, error) {
	ip, err := fetchRemoteIPFrom(gateway, "https://ipinfo.io/ip")
	if err == nil {
		return ip, nil
	}
	fallbackErr := err
	ip, err = fetchRemoteIPFrom(gateway, "https://api.ipify.org")
	if err == nil {
		return ip, nil
	}
	return "", fmt.Errorf("failed to fetch public IP: %v; fallback failed: %v", fallbackErr, err)
}

func runRemoteProxySetup(gateway *gatewayClient, domain string, publicIP string, username string, password string, proxyImage string) error {
	remoteArgs := []string{"proxy", "setup", "--domain", domain, "--public-ip", publicIP}
	if strings.TrimSpace(username) != "" {
		remoteArgs = append(remoteArgs, "--username", username)
	}
	if strings.TrimSpace(password) != "" {
		remoteArgs = append(remoteArgs, "--password-stdin")
	}
	if strings.TrimSpace(proxyImage) != "" {
		remoteArgs = append(remoteArgs, "--proxy-image", proxyImage)
	}
	input := ""
	if strings.TrimSpace(password) != "" {
		input = password + "\n"
	}
	output, err := gateway.command(remoteArgs, input, nil)
	if err != nil {
		trimmed := strings.TrimSpace(err.Error())
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "unknown flag: --domain") ||
			strings.Contains(lower, "unknown flag: --public-ip") ||
			strings.Contains(lower, "unknown flag: --username") ||
			strings.Contains(lower, "unknown flag: --password-stdin") ||
			strings.Contains(lower, "unknown flag: --proxy-image") {
			return fmt.Errorf("remote viberun-server is out of date; rerun setup to update it")
		}
		return err
	}
	if strings.TrimSpace(output) != "" {
		fmt.Fprint(os.Stdout, output)
	}
	return nil
}

func fetchRemoteIPFrom(gateway *gatewayClient, url string) (string, error) {
	output, err := gateway.exec([]string{"curl", "-fsSL", url}, "", nil)
	if err != nil {
		return "", fmt.Errorf("curl %s failed: %v", url, err)
	}
	ip := strings.TrimSpace(output)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("unexpected public IP response: %q", ip)
	}
	return ip, nil
}

func fetchRemoteServerVersion(host string) (string, error) {
	remoteArgs := sshcmd.WithSudo(host, []string{"viberun-server", "version"})
	output, err := runRemoteCommandWithInput(host, remoteArgs, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
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

func fetchProxyInfo(gateway *gatewayClient, app string) (proxyInfo, error) {
	remoteArgs := []string{"proxy", "info", app}
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return proxyInfo{}, err
	}
	var info proxyInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return proxyInfo{}, fmt.Errorf("failed to parse proxy info: %w", err)
	}
	return info, nil
}

func fetchRemoteProxyConfig(gateway *gatewayClient) (proxyConfigSummary, error) {
	remoteArgs := []string{"proxy", "config"}
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return proxyConfigSummary{}, err
	}
	var summary proxyConfigSummary
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		return proxyConfigSummary{}, fmt.Errorf("failed to parse proxy config: %w", err)
	}
	return summary, nil
}

func runRemoteUsersAdd(gateway *gatewayClient, username string, password string) error {
	args := []string{"proxy", "users", "add", "--username", username, "--password-stdin"}
	_, err := gateway.command(args, password+"\n", nil)
	return err
}

func runRemoteUsersRemove(gateway *gatewayClient, username string) error {
	args := []string{"proxy", "users", "remove", "--username", username}
	_, err := gateway.command(args, "", nil)
	return err
}

func runRemoteUsersSetPassword(gateway *gatewayClient, username string, password string) error {
	args := []string{"proxy", "users", "set-password", "--username", username, "--password-stdin"}
	_, err := gateway.command(args, password+"\n", nil)
	return err
}

func runRemoteWipe(gateway *gatewayClient) error {
	args := []string{"wipe"}
	_, err := gateway.command(args, "", nil)
	return err
}

func runRemoteAppsList(gateway *gatewayClient) ([]string, error) {
	return runRemoteAppsListWithArgs(gateway, []string{"apps"})
}

func runRemoteAppsListWithArgs(gateway *gatewayClient, args []string) ([]string, error) {
	output, err := gateway.command(args, "", nil)
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

func runRemoteAppUpdate(gateway *gatewayClient, host string, app string) error {
	app = strings.TrimSpace(app)
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	if isDevRun() || isDevVersion() {
		if err := stageLocalImage(host); err != nil {
			return err
		}
	}
	env := map[string]string{}
	if isDevRun() || isDevVersion() {
		env["VIBERUN_SKIP_IMAGE_PULL"] = "1"
	}
	output, err := gateway.command([]string{app, "update"}, "", env)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, output)
	return nil
}

func runRemoteSetAccess(gateway *gatewayClient, app string, access string) error {
	args := []string{"proxy", "set-access", app, "--access", access}
	_, err := gateway.command(args, "", nil)
	return err
}

func runRemoteSetDomain(gateway *gatewayClient, app string, domain string, clear bool) error {
	args := []string{"proxy", "set-domain", app}
	if clear {
		args = append(args, "--clear")
	} else {
		args = append(args, "--domain", domain)
	}
	_, err := gateway.command(args, "", nil)
	return err
}

func runRemoteSetDisabled(gateway *gatewayClient, app string, disabled bool) error {
	args := []string{"proxy", "set-disabled", app}
	if disabled {
		args = append(args, "--disabled")
	} else {
		args = append(args, "--enabled")
	}
	_, err := gateway.command(args, "", nil)
	return err
}

func runRemoteSetUsers(gateway *gatewayClient, app string, users []string) error {
	joined := strings.Join(users, ",")
	args := []string{"proxy", "set-users", app, "--users", joined}
	_, err := gateway.command(args, "", nil)
	return err
}

func promptRecreateApps(gateway *gatewayClient, host string) error {
	apps, err := runRemoteAppsList(gateway)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stdout, "App URLs changed. Run `app <app>` then `update` to refresh environment variables.")
		return nil
	}
	prompt := fmt.Sprintf("Recreate %d app container(s) to set URL env vars? [Y/n]: ", len(apps))
	if !promptYesNoDefaultYes(prompt) {
		return nil
	}
	for _, app := range apps {
		fmt.Fprintf(os.Stdout, "Updating %s...\n", app)
		if err := runRemoteAppUpdate(gateway, host, app); err != nil {
			return err
		}
	}
	return nil
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
		{cmd: fmt.Sprintf("app %s url set-domain <domain>", info.App), desc: "set a full domain (e.g., myblog.com)"},
		{cmd: fmt.Sprintf("app %s url reset-domain", info.App), desc: "reset to the default domain"},
		{cmd: fmt.Sprintf("app %s users", info.App), desc: "manage who can access this app"},
		{cmd: fmt.Sprintf("app %s url public", info.App), desc: "allow anyone to access"},
		{cmd: fmt.Sprintf("app %s url private", info.App), desc: "require login to access"},
		{cmd: fmt.Sprintf("app %s url disable", info.App), desc: "turn off the URL"},
		{cmd: fmt.Sprintf("app %s url enable", info.App), desc: "turn the URL back on"},
	})
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
	selected := theme.ForOutput(out)
	if !selected.Enabled {
		return urlStyler{}
	}
	return urlStyler{
		enabled:       true,
		labelStyle:    selected.URL.Label,
		valueStyle:    selected.URL.Value,
		headerStyle:   selected.URL.Header,
		commandStyle:  selected.URL.Command,
		commentStyle:  selected.URL.Comment,
		linkStyle:     selected.URL.Link,
		publicStyle:   selected.URL.Public,
		privateStyle:  selected.URL.Private,
		disabledStyle: selected.URL.Disabled,
		ipStyle:       selected.URL.IP,
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

func runUsersEditor(gateway *gatewayClient, app string, info proxyInfo) (bool, error) {
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
		fmt.Fprintln(os.Stdout, "No secondary users configured. Add one with: users add --username <u>")
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
	form.WithInput(os.Stdin).WithOutput(os.Stdout).WithTheme(tui.PromptTheme(os.Stdout))
	if err := form.Run(); err != nil {
		return false, err
	}
	selected = proxy.NormalizeUserList(selected)
	if sameStringSet(selected, filterSecondary(info.AllowedUsers, primary)) {
		return false, nil
	}
	if err := runRemoteSetUsers(gateway, app, selected); err != nil {
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

func printMissingHostMessage() {
	fmt.Fprintln(os.Stderr, "No host is configured yet, so viberun does not know where to connect.")
	fmt.Fprintln(os.Stderr, "Start the shell and run setup:")
	fmt.Fprintln(os.Stderr, "  viberun")
	fmt.Fprintln(os.Stderr, "  setup")
	fmt.Fprintln(os.Stderr, "Or run setup directly:")
	fmt.Fprintln(os.Stderr, "  viberun setup <host>")
}

func maybeClearDefaultAgentOnFailure(cfg config.Config, cfgPath string, agentOverride string, app string, stderrOutput string) {
	if strings.TrimSpace(agentOverride) != "" {
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
		fmt.Fprintf(os.Stderr, "test the agent locally: viberun, then `vibe %s`, then run: %s %s --help\n", app, runner, pkg)
	}
	fmt.Fprintf(os.Stderr, "set it as default with: config set agent %s:%s\n", runner, pkg)
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

if ! need_cmd setfacl; then
  $SUDO apt-get update -y
  $SUDO apt-get install -y acl
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

func resolveHostPort(gateway *gatewayClient, app string, agentProvider string) (int, error) {
	remoteArgs := buildAppCommandArgs(agentProvider, app, []string{"port"})
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve host port: %v", err)
	}
	portText := strings.TrimSpace(string(output))
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("unexpected host port response: %q", portText)
	}
	return port, nil
}

func remoteContainerExists(gateway *gatewayClient, app string, agentProvider string) (bool, error) {
	remoteArgs := buildAppCommandArgs(agentProvider, app, []string{"exists"})
	output, err := gateway.command(remoteArgs, "", nil)
	if err != nil {
		return false, fmt.Errorf("failed to check container: %v", err)
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
	if devServerSync.skip == nil {
		devServerSync.skip = map[string]bool{}
	}
	if devServerSync.hosts[host] {
		devServerSync.mu.Unlock()
		return nil
	}
	if devServerSync.skip[host] {
		devServerSync.mu.Unlock()
		return nil
	}
	devServerSync.mu.Unlock()

	if !shouldSyncDevServer(host) {
		devServerSync.mu.Lock()
		devServerSync.skip[host] = true
		devServerSync.mu.Unlock()
		return nil
	}

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

func shouldSyncDevServer(host string) bool {
	if value := strings.TrimSpace(os.Getenv("VIBERUN_DEV_SYNC")); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	}
	return false
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
