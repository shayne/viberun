// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package authbundle

// Bundle describes auth data staged on the host and ready to be copied into a container.
type Bundle struct {
	Provider string            `json:"provider"`
	Files    []File            `json:"files,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}

// File is a host file that should be copied into the container.
type File struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	Mode          int    `json:"mode,omitempty"`
}
