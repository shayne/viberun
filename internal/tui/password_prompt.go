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

func PromptPassword(in io.Reader, out io.Writer, title string) (string, error) {
	password := ""
	if strings.TrimSpace(title) == "" {
		title = "Password"
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
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
		return "", err
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.New("password is required")
	}
	return password, nil
}
