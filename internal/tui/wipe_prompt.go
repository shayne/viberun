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
				Description("This deletes local config and all viberun data on the host.").
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
	form.WithInput(in).WithOutput(out).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return err
	}
	if strings.TrimSpace(value) != "WIPE" {
		return errors.New("confirmation did not match")
	}
	return nil
}
