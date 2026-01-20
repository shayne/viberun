// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package containerconfig

// Config describes per-app container settings provided by the host.
type Config struct {
	App     AppConfig     `json:"app"`
	Ports   PortConfig    `json:"ports"`
	HostRPC HostRPCConfig `json:"hostrpc"`
	Proxy   *ProxyConfig  `json:"proxy,omitempty"`
}

// AppConfig identifies the app and container.
type AppConfig struct {
	Name      string `json:"name,omitempty"`
	Container string `json:"container,omitempty"`
}

// PortConfig holds app/host ports.
type PortConfig struct {
	App  int `json:"app,omitempty"`
	Host int `json:"host,omitempty"`
	Web  int `json:"web,omitempty"`
}

// HostRPCConfig points to the host RPC socket and token file inside the container.
type HostRPCConfig struct {
	Socket    string `json:"socket,omitempty"`
	TokenFile string `json:"token_file,omitempty"`
}

// ProxyConfig holds public URL details plus optional env var names.
type ProxyConfig struct {
	PublicURL       string `json:"public_url,omitempty"`
	PublicDomain    string `json:"public_domain,omitempty"`
	PublicURLEnv    string `json:"public_url_env,omitempty"`
	PublicDomainEnv string `json:"public_domain_env,omitempty"`
}
