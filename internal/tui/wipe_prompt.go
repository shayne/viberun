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

func PromptWipeConfirm(in io.Reader, out io.Writer) error {
	value := ""
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Type WIPE to confirm").
				Description("This permanently removes all viberun data from the server. This cannot be undone.").
				Placeholder("WIPE").
				Value(&value).
				Validate(func(input string) error {
					if strings.TrimSpace(input) != "WIPE" {
						return errors.New("type WIPE to confirm")
					}
					return nil
				}),
		),
	)
	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))
	if err := form.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(value) != "WIPE" {
		return errors.New("confirmation did not match")
	}
	return nil
}

func PromptWipeDecision(in io.Reader, out io.Writer, host string) (bool, error) {
	value := false
	host = strings.TrimSpace(host)
	title := "Wipe this server?"
	if host != "" {
		title = "Wipe " + host + "?"
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description("This removes all viberun data, containers, and configuration from that server.").
				Value(&value),
		),
	)
	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))
	if err := form.Run(); err != nil {
		return false, err
	}
	return value, nil
}
