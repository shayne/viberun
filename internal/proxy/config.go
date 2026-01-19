// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultAdminAddr      = "127.0.0.1:2019"
	defaultCaddyContainer = "viberun-proxy"
	defaultConfigPath     = "/var/lib/viberun/proxy.toml"
	defaultProxyImage     = "ghcr.io/shayne/viberun/viberun-proxy:latest"
	defaultAccessMode     = "private"
	defaultAuthListenAddr = "127.0.0.1:9180"
	defaultAuthCookieName = "viberun_auth"
	defaultAuthCookieTTL  = "12h"
	defaultCookieSameSite = "lax"
	defaultEnvPublicURL   = "VIBERUN_PUBLIC_URL"
	defaultEnvDomain      = "VIBERUN_PUBLIC_DOMAIN"
)

type AppAccess struct {
	Access       string   `toml:"access"`
	AllowedUsers []string `toml:"allowed_users"`
	Disabled     bool     `toml:"disabled"`
	CustomDomain string   `toml:"custom_domain"`
}

type AuthConfig struct {
	ListenAddr   string `toml:"listen_addr"`
	SigningKey   string `toml:"signing_key"`
	CookieName   string `toml:"cookie_name"`
	CookieTTL    string `toml:"cookie_ttl"`
	CookieDomain string `toml:"cookie_domain"`
	CookieSecure *bool  `toml:"cookie_secure"`
	CookieSame   string `toml:"cookie_samesite"`
}

type EnvConfig struct {
	PublicURL    string `toml:"public_url"`
	PublicDomain string `toml:"public_domain"`
}

type AuthUser struct {
	Username string `toml:"username"`
	Email    string `toml:"email"`
	Password string `toml:"password"`
}

type Config struct {
	Enabled        bool                 `toml:"enabled"`
	BaseDomain     string               `toml:"base_domain"`
	PublicIP       string               `toml:"public_ip"`
	AdminAddr      string               `toml:"admin_addr"`
	CaddyContainer string               `toml:"caddy_container"`
	ProxyImage     string               `toml:"proxy_image"`
	PrimaryUser    string               `toml:"primary_user"`
	DefaultAccess  string               `toml:"default_access"`
	Apps           map[string]AppAccess `toml:"apps"`
	Users          []AuthUser           `toml:"users"`
	Auth           AuthConfig           `toml:"auth"`
	Env            EnvConfig            `toml:"env"`
}

func ConfigPath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("VIBERUN_PROXY_CONFIG_PATH")); value != "" {
		return value, nil
	}
	return defaultConfigPath, nil
}

func LoadConfig() (Config, string, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := loadConfigFromPath(path)
	if err != nil {
		return Config{}, path, err
	}
	return cfg, path, nil
}

func LoadConfigFromPath(path string) (Config, error) {
	return loadConfigFromPath(path)
}

func SaveConfig(path string, cfg Config) error {
	cfg = applyDefaults(cfg)
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadConfigFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return applyDefaults(Config{}), nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return applyDefaults(cfg), nil
}

func applyDefaults(cfg Config) Config {
	if strings.TrimSpace(cfg.AdminAddr) == "" {
		cfg.AdminAddr = defaultAdminAddr
	}
	if strings.TrimSpace(cfg.CaddyContainer) == "" {
		cfg.CaddyContainer = defaultCaddyContainer
	}
	if strings.TrimSpace(cfg.ProxyImage) == "" {
		if value := strings.TrimSpace(os.Getenv("VIBERUN_PROXY_IMAGE")); value != "" {
			cfg.ProxyImage = value
		} else {
			cfg.ProxyImage = defaultProxyImage
		}
	}
	if strings.TrimSpace(cfg.DefaultAccess) == "" {
		cfg.DefaultAccess = defaultAccessMode
	}
	if cfg.Apps == nil {
		cfg.Apps = map[string]AppAccess{}
	}
	if strings.TrimSpace(cfg.Auth.ListenAddr) == "" {
		cfg.Auth.ListenAddr = defaultAuthListenAddr
	}
	if strings.TrimSpace(cfg.Auth.CookieName) == "" {
		cfg.Auth.CookieName = defaultAuthCookieName
	}
	if strings.TrimSpace(cfg.Auth.CookieTTL) == "" {
		cfg.Auth.CookieTTL = defaultAuthCookieTTL
	}
	if strings.TrimSpace(cfg.Auth.CookieSame) == "" {
		cfg.Auth.CookieSame = defaultCookieSameSite
	}
	if strings.TrimSpace(cfg.Env.PublicURL) == "" {
		cfg.Env.PublicURL = defaultEnvPublicURL
	}
	if strings.TrimSpace(cfg.Env.PublicDomain) == "" {
		cfg.Env.PublicDomain = defaultEnvDomain
	}
	cfg.BaseDomain = strings.TrimSpace(cfg.BaseDomain)
	cfg.PublicIP = strings.TrimSpace(cfg.PublicIP)
	cfg.PrimaryUser = strings.TrimSpace(cfg.PrimaryUser)
	cfg.DefaultAccess = strings.TrimSpace(cfg.DefaultAccess)
	cfg.Users = normalizeUsers(cfg.Users)
	for name, app := range cfg.Apps {
		app.Access = strings.TrimSpace(app.Access)
		app.CustomDomain = strings.TrimSpace(app.CustomDomain)
		app.AllowedUsers = normalizeUserList(app.AllowedUsers)
		cfg.Apps[name] = app
	}
	cfg.Auth.ListenAddr = strings.TrimSpace(cfg.Auth.ListenAddr)
	cfg.Auth.SigningKey = strings.TrimSpace(cfg.Auth.SigningKey)
	cfg.Auth.CookieName = strings.TrimSpace(cfg.Auth.CookieName)
	cfg.Auth.CookieTTL = strings.TrimSpace(cfg.Auth.CookieTTL)
	cfg.Auth.CookieDomain = strings.TrimSpace(cfg.Auth.CookieDomain)
	cfg.Auth.CookieSame = strings.TrimSpace(cfg.Auth.CookieSame)
	return cfg
}

func DefaultAdminAddr() string {
	return defaultAdminAddr
}

func DefaultCaddyContainer() string {
	return defaultCaddyContainer
}

func DefaultProxyImage() string {
	return defaultProxyImage
}

func DefaultAccessMode() string {
	return defaultAccessMode
}

func DefaultAuthListenAddr() string {
	return defaultAuthListenAddr
}

func DefaultAuthCookieName() string {
	return defaultAuthCookieName
}

func DefaultAuthCookieTTL() string {
	return defaultAuthCookieTTL
}

func DefaultEnvPublicURL() string {
	return defaultEnvPublicURL
}

func DefaultEnvPublicDomain() string {
	return defaultEnvDomain
}

func normalizeUserList(users []string) []string {
	if len(users) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(users))
	for _, user := range users {
		user = strings.TrimSpace(user)
		if user == "" || seen[user] {
			continue
		}
		seen[user] = true
		out = append(out, user)
	}
	return out
}

func NormalizeUserList(users []string) []string {
	return normalizeUserList(users)
}

func normalizeUsers(users []AuthUser) []AuthUser {
	if len(users) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]AuthUser, 0, len(users))
	for _, user := range users {
		user.Username = strings.TrimSpace(user.Username)
		user.Email = strings.TrimSpace(user.Email)
		user.Password = strings.TrimSpace(user.Password)
		if user.Username == "" || seen[user.Username] {
			continue
		}
		seen[user.Username] = true
		if user.Email == "" {
			user.Email = user.Username + "@local"
		}
		out = append(out, user)
	}
	return out
}
