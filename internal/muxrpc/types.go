// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package muxrpc

import "encoding/json"

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type CommandParams struct {
	Args  []string          `json:"args"`
	Input string            `json:"input,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
	TTY   bool              `json:"tty,omitempty"`
}

type CommandResult struct {
	Output string `json:"output"`
}

type ExecParams struct {
	Args  []string          `json:"args"`
	Input string            `json:"input,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
}

type ExecResult struct {
	Output string `json:"output"`
}

type PtyMeta struct {
	App    string            `json:"app"`
	Action string            `json:"action,omitempty"`
	Agent  string            `json:"agent,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

type ForwardMeta struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port"`
}

type UploadMeta struct {
	Target    string `json:"target"`
	Path      string `json:"path"`
	Container string `json:"container,omitempty"`
	Mode      int    `json:"mode,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Sized     bool   `json:"sized,omitempty"`
}

type UploadResult struct {
	Error string `json:"error,omitempty"`
}

type OpenEvent struct {
	URL string `json:"url"`
}

type ResizeEvent struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

type AppSnapshot struct {
	Name string `json:"name"`
	Port int    `json:"port,omitempty"`
}

type AppsEvent struct {
	Apps []AppSnapshot `json:"apps"`
}
