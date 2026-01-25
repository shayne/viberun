// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"strings"
)

func PromptPassword(in io.Reader, out io.Writer, title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		title = "Password"
	}
	if useDialogPrompts(in, out) {
		return promptInputDialog(in, out, title, "", "", "", func(input string) error {
			if strings.TrimSpace(input) == "" {
				return errors.New("password is required")
			}
			return nil
		}, true)
	}
	value, err := promptInput(in, out, title, "", "", func(input string) error {
		if strings.TrimSpace(input) == "" {
			return errors.New("password is required")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("password is required")
	}
	return value, nil
}
