// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func PromptPassword(in io.Reader, out io.Writer, title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		title = "Password"
	}
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprintln(out, title)
		fmt.Fprint(out, "> ")
		value, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		password := strings.TrimSpace(string(value))
		if password == "" {
			return "", errors.New("password is required")
		}
		return password, nil
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
