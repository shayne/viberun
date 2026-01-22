// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
)

func PromptSetupHost(in io.Reader, out io.Writer, defaultHost string) (string, error) {
	value := strings.TrimSpace(defaultHost)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Server login").
				Placeholder("user@host  # username + address").
				Value(&value).
				Validate(func(input string) error {
					if strings.TrimSpace(input) == "" {
						return errors.New("please enter a server login")
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

func PromptSetupRerun(in io.Reader, out io.Writer, host string) (bool, error) {
	value := false
	title := "You're already connected."
	host = strings.TrimSpace(host)
	if host != "" {
		title = "You're already connected to " + host + "."
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description("Set up a different server?").
				Value(&value),
		),
	)
	form.WithInput(in).WithOutput(out).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return false, err
	}
	return value, nil
}
