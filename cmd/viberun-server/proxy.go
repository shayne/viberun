// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/shayne/viberun/internal/hostcmd"
	"github.com/shayne/viberun/internal/proxy"
	"github.com/shayne/viberun/internal/server"
	"github.com/shayne/yargs"
)

var errProxyUnavailable = errors.New("proxy is not configured")

type proxySetupFlags struct {
	Domain         string `flag:"domain" help:"base domain for app URLs"`
	PublicIP       string `flag:"public-ip" help:"public IP address for DNS"`
	AdminAddr      string `flag:"admin-addr" help:"caddy admin address"`
	CaddyContainer string `flag:"caddy-container" help:"caddy container name"`
	ProxyImage     string `flag:"proxy-image" help:"proxy container image"`
	Username       string `flag:"username" help:"primary auth username"`
	PasswordStdin  bool   `flag:"password-stdin" help:"read password from stdin"`
	AuthKey        string `flag:"auth-key" help:"auth signing key (optional)"`
}

func handleProxyCommand(args []string) error {
	if len(args) == 0 || hasHelpFlag(args) {
		return fmt.Errorf("usage: viberun-server proxy setup --domain <domain> --public-ip <ip> --username <u> --password-stdin | viberun-server proxy url <app> | viberun-server proxy info <app> | viberun-server proxy users <list|add|remove|set-password>")
	}
	switch args[0] {
	case "setup":
		return handleProxySetup(args[1:])
	case "url":
		return handleProxyURL(args[1:])
	case "info":
		return handleProxyInfo(args[1:])
	case "set-access":
		return handleProxySetAccess(args[1:])
	case "set-domain":
		return handleProxySetDomain(args[1:])
	case "set-users":
		return handleProxySetUsers(args[1:])
	case "set-disabled":
		return handleProxySetDisabled(args[1:])
	case "users":
		return handleProxyUsers(args[1:])
	default:
		return fmt.Errorf("usage: viberun-server proxy setup --domain <domain> --public-ip <ip> --username <u> --password-stdin | viberun-server proxy url <app> | viberun-server proxy info <app> | viberun-server proxy users <list|add|remove|set-password>")
	}
}

func handleProxySetup(args []string) error {
	result, err := yargs.ParseFlags[proxySetupFlags](args)
	if err != nil {
		return err
	}
	flags := result.Flags
	domain, err := proxy.NormalizeDomainSuffix(flags.Domain)
	if err != nil {
		return err
	}
	publicIP := strings.TrimSpace(flags.PublicIP)
	if publicIP == "" {
		return fmt.Errorf("public IP is required")
	}

	cfg, _, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Enabled = true
	cfg.BaseDomain = domain
	cfg.PublicIP = publicIP
	cfg.AdminAddr = strings.TrimSpace(flags.AdminAddr)
	cfg.CaddyContainer = strings.TrimSpace(flags.CaddyContainer)
	if strings.TrimSpace(flags.ProxyImage) != "" {
		cfg.ProxyImage = strings.TrimSpace(flags.ProxyImage)
	}
	if strings.TrimSpace(flags.Username) != "" {
		cfg.PrimaryUser = strings.TrimSpace(flags.Username)
	}
	if strings.TrimSpace(flags.AuthKey) != "" {
		cfg.Auth.SigningKey = strings.TrimSpace(flags.AuthKey)
	}
	if strings.TrimSpace(cfg.Auth.SigningKey) == "" {
		key, err := proxy.GenerateSigningKey()
		if err != nil {
			return err
		}
		cfg.Auth.SigningKey = key
	}
	password, err := readPasswordIfRequested(flags.PasswordStdin)
	if err != nil {
		return err
	}
	if password != "" {
		if strings.TrimSpace(cfg.PrimaryUser) == "" {
			return fmt.Errorf("username is required when setting a password")
		}
		if err := proxy.UpsertUser(&cfg, cfg.PrimaryUser, password); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.PrimaryUser) == "" {
		return fmt.Errorf("primary username is required")
	}
	if len(cfg.Users) == 0 {
		return fmt.Errorf("at least one user is required")
	}

	cfg = applyProxyDefaults(cfg)

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required but was not found in PATH")
	}
	if err := ensureRootfulDocker(); err != nil {
		return err
	}

	running, err := caddyContainerRunning(cfg.CaddyContainer)
	if err != nil {
		return err
	}
	if !running {
		if err := checkPortsAvailable([]int{80, 443}); err != nil {
			return err
		}
	}

	path, err := proxy.ConfigPath()
	if err != nil {
		return err
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	if err := ensureCaddyContainer(cfg.CaddyContainer, cfg.ProxyImage); err != nil {
		return err
	}

	state, _, err := server.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load server state: %w", err)
	}
	if err := syncProxyWithState(cfg, state); err != nil {
		return err
	}
	return nil
}

func handleProxyURL(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: viberun-server proxy url <app>")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	url, err := proxyURLForApp(app)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, url)
	return nil
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

func handleProxyInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: viberun-server proxy info <app>")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	cfg, _, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseDomain) == "" {
		return errProxyUnavailable
	}
	access := proxy.EffectiveAppAccess(cfg, app)
	info := proxyInfo{
		App:          app,
		URL:          proxy.PublicURLForApp(cfg, app),
		Access:       access.Access,
		Disabled:     access.Disabled,
		CustomDomain: access.CustomDomain,
		AllowedUsers: proxy.EffectiveAllowedUsers(cfg, app),
		PrimaryUser:  cfg.PrimaryUser,
		Users:        proxy.Usernames(cfg),
		BaseDomain:   cfg.BaseDomain,
		PublicIP:     cfg.PublicIP,
		Enabled:      cfg.Enabled,
	}
	encoder := json.NewEncoder(os.Stdout)
	return encoder.Encode(info)
}

type proxyAccessFlags struct {
	Access string `flag:"access" help:"public or private"`
}

func handleProxySetAccess(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: viberun-server proxy set-access <app> --access <public|private>")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	result, err := yargs.ParseFlags[proxyAccessFlags](args[1:])
	if err != nil {
		return err
	}
	access := proxy.NormalizeAccessMode(result.Flags.Access)
	if access != proxy.AccessPublic && access != proxy.AccessPrivate {
		return fmt.Errorf("access must be public or private")
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if err := setProxyAccess(&cfg, app, access); err != nil {
		return err
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

type proxyDomainFlags struct {
	Domain string `flag:"domain" help:"custom full domain"`
	Clear  bool   `flag:"clear" help:"clear custom domain"`
}

func handleProxySetDomain(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: viberun-server proxy set-domain <app> --domain <domain> | --clear")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	result, err := yargs.ParseFlags[proxyDomainFlags](args[1:])
	if err != nil {
		return err
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	appCfg := cfg.Apps[app]
	if result.Flags.Clear {
		appCfg.CustomDomain = ""
	} else {
		domain := strings.TrimSpace(result.Flags.Domain)
		if domain == "" {
			return fmt.Errorf("domain is required")
		}
		normalized, err := proxy.NormalizeDomainSuffix(domain)
		if err != nil {
			return err
		}
		appCfg.CustomDomain = normalized
	}
	cfg.Apps[app] = appCfg
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

type proxyUsersFlags struct {
	Users string `flag:"users" help:"comma-separated list of secondary users"`
}

func handleProxySetUsers(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: viberun-server proxy set-users <app> --users <user1,user2>")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	result, err := yargs.ParseFlags[proxyUsersFlags](args[1:])
	if err != nil {
		return err
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	secondary := parseUsersList(result.Flags.Users, cfg.PrimaryUser)
	if err := validateUsersExist(cfg, secondary); err != nil {
		return err
	}
	appCfg := cfg.Apps[app]
	appCfg.AllowedUsers = secondary
	cfg.Apps[app] = appCfg
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

type proxyDisabledFlags struct {
	Disabled bool `flag:"disabled" help:"disable public URL"`
	Enabled  bool `flag:"enabled" help:"enable public URL"`
}

func handleProxySetDisabled(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: viberun-server proxy set-disabled <app> --disabled|--enabled")
	}
	app := strings.TrimSpace(args[0])
	if app == "" {
		return fmt.Errorf("app name is required")
	}
	result, err := yargs.ParseFlags[proxyDisabledFlags](args[1:])
	if err != nil {
		return err
	}
	if result.Flags.Disabled && result.Flags.Enabled {
		return fmt.Errorf("choose either --disabled or --enabled")
	}
	disabled := result.Flags.Disabled
	if result.Flags.Enabled {
		disabled = false
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	setProxyDisabled(&cfg, app, disabled)
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

func setProxyAccess(cfg *proxy.Config, app string, access string) error {
	if cfg == nil {
		return fmt.Errorf("proxy config is nil")
	}
	if strings.TrimSpace(app) == "" {
		return fmt.Errorf("app name is required")
	}
	normalized := proxy.NormalizeAccessMode(access)
	if normalized != proxy.AccessPublic && normalized != proxy.AccessPrivate {
		return fmt.Errorf("access must be public or private")
	}
	appCfg := cfg.Apps[app]
	appCfg.Access = normalized
	appCfg.Disabled = false
	cfg.Apps[app] = appCfg
	return nil
}

func setProxyDisabled(cfg *proxy.Config, app string, disabled bool) {
	if cfg == nil || strings.TrimSpace(app) == "" {
		return
	}
	appCfg := cfg.Apps[app]
	appCfg.Disabled = disabled
	cfg.Apps[app] = appCfg
}

type proxyUserFlags struct {
	Username      string `flag:"username" help:"username"`
	PasswordStdin bool   `flag:"password-stdin" help:"read password from stdin"`
}

func handleProxyUsers(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: viberun-server proxy users <list|add|remove|set-password>")
	}
	switch args[0] {
	case "list":
		return handleProxyUsersList()
	case "add":
		return handleProxyUsersAdd(args[1:])
	case "remove":
		return handleProxyUsersRemove(args[1:])
	case "set-password":
		return handleProxyUsersSetPassword(args[1:])
	default:
		return fmt.Errorf("usage: viberun-server proxy users <list|add|remove|set-password>")
	}
}

func handleProxyUsersList() error {
	cfg, _, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	for _, user := range proxy.Usernames(cfg) {
		fmt.Fprintln(os.Stdout, user)
	}
	return nil
}

func handleProxyUsersAdd(args []string) error {
	result, err := yargs.ParseFlags[proxyUserFlags](args)
	if err != nil {
		return err
	}
	username := strings.TrimSpace(result.Flags.Username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	password, err := readPasswordIfRequested(result.Flags.PasswordStdin)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if err := proxy.UpsertUser(&cfg, username, password); err != nil {
		return err
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

func handleProxyUsersRemove(args []string) error {
	result, err := yargs.ParseFlags[proxyUserFlags](args)
	if err != nil {
		return err
	}
	username := strings.TrimSpace(result.Flags.Username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if strings.EqualFold(cfg.PrimaryUser, username) {
		return fmt.Errorf("cannot remove the primary user")
	}
	removed, err := proxy.RemoveUser(&cfg, username)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("user not found")
	}
	removeUserFromApps(&cfg, username)
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

func handleProxyUsersSetPassword(args []string) error {
	result, err := yargs.ParseFlags[proxyUserFlags](args)
	if err != nil {
		return err
	}
	username := strings.TrimSpace(result.Flags.Username)
	if username == "" {
		return fmt.Errorf("username is required")
	}
	password, err := readPasswordIfRequested(result.Flags.PasswordStdin)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	cfg, path, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if err := proxy.UpsertUser(&cfg, username, password); err != nil {
		return err
	}
	if err := proxy.SaveConfig(path, cfg); err != nil {
		return err
	}
	state, err := loadState()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

func proxyURLForApp(app string) (string, error) {
	cfg, _, err := proxy.LoadConfig()
	if err != nil {
		return "", err
	}
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseDomain) == "" {
		return "", errProxyUnavailable
	}
	url := proxy.PublicURLForApp(cfg, app)
	if url == "" {
		return "", errProxyUnavailable
	}
	return url, nil
}

func syncProxyFromState(state server.State) error {
	cfg, _, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	return syncProxyWithState(cfg, state)
}

func warnProxySync(state server.State) {
	if err := syncProxyFromState(state); err != nil && !errors.Is(err, errProxyUnavailable) {
		fmt.Fprintf(os.Stderr, "warning: failed to sync proxy: %v\n", err)
	}
}

func syncProxyWithState(cfg proxy.Config, state server.State) error {
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseDomain) == "" {
		return nil
	}
	if err := ensureCaddyContainer(cfg.CaddyContainer, cfg.ProxyImage); err != nil {
		return err
	}
	caddyCfg, err := proxy.BuildCaddyConfig(cfg, state.Ports)
	if err != nil {
		return err
	}
	return postCaddyConfig(cfg.AdminAddr, caddyCfg)
}

func applyProxyDefaults(cfg proxy.Config) proxy.Config {
	if strings.TrimSpace(cfg.AdminAddr) == "" {
		cfg.AdminAddr = proxy.DefaultAdminAddr()
	}
	if strings.TrimSpace(cfg.CaddyContainer) == "" {
		cfg.CaddyContainer = proxy.DefaultCaddyContainer()
	}
	if strings.TrimSpace(cfg.ProxyImage) == "" {
		cfg.ProxyImage = proxy.DefaultProxyImage()
	}
	return cfg
}

func ensureCaddyContainer(name string, image string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = proxy.DefaultCaddyContainer()
	}
	exists, running, err := caddyContainerStatus(name)
	if err != nil {
		return err
	}
	if exists {
		if strings.TrimSpace(image) == "" {
			image = proxy.DefaultProxyImage()
		}
		if imageID, err := dockerImageID(image); err == nil && imageID != "" {
			if currentID, err := containerImageID(name); err == nil && currentID != "" && currentID != imageID {
				_ = runDockerCommandOutput("rm", "-f", name)
				exists = false
				running = false
			}
		}
	}
	if !exists {
		if strings.TrimSpace(image) == "" {
			image = proxy.DefaultProxyImage()
		}
		if err := runDockerCommandOutput("run", "-d", "--name", name, "--network", "host", "--restart", "unless-stopped", "-v", "/var/lib/viberun/caddy:/data", "-v", "/var/lib/viberun/caddy:/config", "-v", "/var/lib/viberun/proxy.toml:/var/lib/viberun/proxy.toml:ro", image); err != nil {
			message := err.Error()
			if strings.Contains(message, "pull access denied") || strings.Contains(message, "not found") || strings.Contains(message, "manifest unknown") {
				return fmt.Errorf("proxy image %s not available; re-run bootstrap or pull it manually: %s", image, err)
			}
			return fmt.Errorf("failed to start caddy container: %w", err)
		}
		return nil
	}
	if !running {
		if err := runDockerCommandOutput("start", name); err != nil {
			return fmt.Errorf("failed to start caddy container: %w", err)
		}
	}
	return nil
}

func dockerImageID(image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", nil
	}
	out, err := hostcmd.RunCapture("docker", "image", "inspect", "-f", "{{.Id}}", image)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func caddyContainerRunning(name string) (bool, error) {
	_, running, err := caddyContainerStatus(name)
	return running, err
}

func caddyContainerStatus(name string) (bool, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = proxy.DefaultCaddyContainer()
	}
	out, err := hostcmd.RunCapture("docker", "inspect", "-f", "{{.State.Running}}", name)
	if err != nil {
		if strings.Contains(err.Error(), "No such object") {
			return false, false, nil
		}
		return false, false, err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return false, false, nil
	}
	switch strings.ToLower(trimmed) {
	case "true":
		return true, true, nil
	case "false":
		return true, false, nil
	default:
		return true, false, nil
	}
}

func postCaddyConfig(adminAddr string, cfg proxy.CaddyConfig) error {
	adminAddr = strings.TrimSpace(adminAddr)
	if adminAddr == "" {
		adminAddr = proxy.DefaultAdminAddr()
	}
	url := "http://" + adminAddr + "/load"
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 10; i++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(cfg.Body))
		if err != nil {
			return err
		}
		contentType := strings.TrimSpace(cfg.ContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := client.Do(req)
		if err == nil {
			payload, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			return fmt.Errorf("caddy admin returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("failed to reach caddy admin API at %s", url)
}

func checkPortsAvailable(ports []int) error {
	sort.Ints(ports)
	var inUse []string
	for _, port := range ports {
		used, detail, err := portInUse(port)
		if err != nil {
			return err
		}
		if used {
			if detail != "" {
				inUse = append(inUse, fmt.Sprintf("port %d: %s", port, detail))
			} else {
				inUse = append(inUse, fmt.Sprintf("port %d is in use", port))
			}
		}
	}
	if len(inUse) == 0 {
		return nil
	}
	return fmt.Errorf("ports 80 and/or 443 are already in use. Stop the process or container bound to those ports and retry. Details: %s", strings.Join(inUse, "; "))
}

func portInUse(port int) (bool, string, error) {
	if _, err := exec.LookPath("ss"); err == nil {
		return portInUseWithSS(port)
	}
	if _, err := exec.LookPath("netstat"); err == nil {
		return portInUseWithNetstat(port)
	}
	return false, "", fmt.Errorf("cannot check ports: ss or netstat is required")
}

func portInUseWithSS(port int) (bool, string, error) {
	filter := []string{"sport", "=", fmt.Sprintf(":%d", port)}
	out, err := hostcmd.RunCapture("ss", append([]string{"-ltnH"}, filter...)...)
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(out) == "" {
		return false, "", nil
	}
	detail, _ := hostcmd.RunCapture("ss", append([]string{"-ltnpH"}, filter...)...)
	return true, summarizePortDetail(detail), nil
}

func portInUseWithNetstat(port int) (bool, string, error) {
	out, err := hostcmd.RunCapture("netstat", "-ltnp")
	if err != nil {
		return false, "", err
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if strings.HasSuffix(fields[3], fmt.Sprintf(":%d", port)) {
			return true, strings.Join(fields, " "), nil
		}
	}
	return false, "", nil
}

func summarizePortDetail(detail string) string {
	trimmed := strings.TrimSpace(detail)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	line := strings.TrimSpace(lines[0])
	if line == "" {
		return ""
	}
	if idx := strings.Index(line, "users:("); idx >= 0 {
		return strings.TrimSpace(line[idx:])
	}
	return line
}

func readPasswordIfRequested(enabled bool) (string, error) {
	if !enabled {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	return password, nil
}

func loadState() (server.State, error) {
	state, _, err := server.LoadState()
	if err != nil {
		return server.State{}, fmt.Errorf("failed to load server state: %w", err)
	}
	return state, nil
}

func parseUsersList(raw string, primary string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	users := make([]string, 0, len(parts))
	for _, part := range parts {
		user := strings.TrimSpace(part)
		if user == "" || strings.EqualFold(user, primary) {
			continue
		}
		users = append(users, user)
	}
	return proxy.NormalizeUserList(users)
}

func validateUsersExist(cfg proxy.Config, users []string) error {
	if len(users) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, user := range proxy.Usernames(cfg) {
		known[user] = true
	}
	for _, user := range users {
		if !known[user] {
			return fmt.Errorf("unknown user %q", user)
		}
	}
	return nil
}

func removeUserFromApps(cfg *proxy.Config, username string) {
	if cfg == nil {
		return
	}
	for name, app := range cfg.Apps {
		if len(app.AllowedUsers) == 0 {
			continue
		}
		filtered := make([]string, 0, len(app.AllowedUsers))
		for _, user := range app.AllowedUsers {
			if user == username {
				continue
			}
			filtered = append(filtered, user)
		}
		app.AllowedUsers = filtered
		cfg.Apps[name] = app
	}
}
