// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/shayne/viberun/internal/containerconfig"
	"github.com/shayne/viberun/internal/proxy"
)

const containerConfigContainerPath = "/opt/viberun/containerconfig.json"

func containerConfigHostPath(app string) string {
	cfg := homeVolumeConfigForApp(app)
	return filepath.Join(cfg.BaseDir, "containerconfig.json")
}

func ensureContainerConfig(app string, containerName string, port int) error {
	cfg, err := buildContainerConfig(app, containerName, port)
	if err != nil {
		return err
	}
	return writeContainerConfig(app, cfg)
}

func buildContainerConfig(app string, containerName string, port int) (containerconfig.Config, error) {
	hostRPC := hostRPCConfigForApp(app)
	cfg := containerconfig.Config{
		App: containerconfig.AppConfig{
			Name:      app,
			Container: containerName,
		},
		Ports: containerconfig.PortConfig{
			App:  8080,
			Host: port,
			Web:  8080,
		},
		HostRPC: containerconfig.HostRPCConfig{
			Socket:    hostRPC.ContainerSocket,
			TokenFile: hostRPC.ContainerTokenFile,
		},
	}
	proxyCfg, _, err := proxy.LoadConfig()
	if err != nil {
		return cfg, err
	}
	if proxyCfg.Enabled && strings.TrimSpace(proxyCfg.BaseDomain) != "" {
		host := proxy.PublicHostForApp(proxyCfg, app)
		url := proxy.PublicURLForApp(proxyCfg, app)
		if host != "" && url != "" {
			cfg.Proxy = &containerconfig.ProxyConfig{
				PublicURL:       url,
				PublicDomain:    host,
				PublicURLEnv:    strings.TrimSpace(proxyCfg.Env.PublicURL),
				PublicDomainEnv: strings.TrimSpace(proxyCfg.Env.PublicDomain),
			}
		}
	}
	return cfg, nil
}

func writeContainerConfig(app string, cfg containerconfig.Config) error {
	path := containerConfigHostPath(app)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
