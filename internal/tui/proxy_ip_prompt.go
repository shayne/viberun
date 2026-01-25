// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"net"
	"strings"
)

func PromptProxyPublicIP(in io.Reader, out io.Writer, defaultIP string) (string, error) {
	input, err := promptInputWithDefault(in, out, "Public IP address", "Used for DNS A records. Leave as-is if unchanged.", "", strings.TrimSpace(defaultIP), func(input string) error {
		ip := net.ParseIP(strings.TrimSpace(input))
		if ip == nil {
			return errors.New("valid IP address is required")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}
