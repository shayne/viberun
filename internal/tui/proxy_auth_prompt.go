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

func PromptProxyAuth(in io.Reader, out io.Writer, defaultUser string) (string, string, error) {
	username := strings.TrimSpace(defaultUser)
	password := ""
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Primary username").
				Prompt("> ").
				Value(&username).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("username is required")
					}
					if strings.ContainsAny(value, " \t") {
						return errors.New("username must not contain spaces")
					}
					return nil
				}),
			huh.NewInput().
				Title("Password").
				Prompt("> ").
				EchoMode(huh.EchoModePassword).
				Value(&password).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("password is required")
					}
					return nil
				}),
		),
	)
	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))
	if err := form.Run(); err != nil {
		return "", "", err
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return "", "", errors.New("username and password are required")
	}
	return username, password, nil
}
