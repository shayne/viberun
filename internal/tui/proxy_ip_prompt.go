// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"net"
	"strings"

	"github.com/charmbracelet/huh"
)

func PromptProxyPublicIP(in io.Reader, out io.Writer, defaultIP string) (string, error) {
	value := strings.TrimSpace(defaultIP)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Public IP address").
				Description("Used for DNS A records. Leave as-is if unchanged.").
				Prompt("> ").
				Value(&value).
				Validate(func(input string) error {
					ip := net.ParseIP(strings.TrimSpace(input))
					if ip == nil {
						return errors.New("valid IP address is required")
					}
					return nil
				}),
		),
	)

	form.WithInput(in).WithOutput(out).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
