// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"strings"
)

func PromptSetupHost(in io.Reader, out io.Writer, defaultHost string) (string, error) {
	input, err := promptInput(in, out, "Server login", "", "user@host  # username + address", func(input string) error {
		if strings.TrimSpace(input) == "" {
			return errors.New("please enter a server login")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(input) == "" {
		return "", errors.New("please enter a server login")
	}
	return strings.TrimSpace(input), nil
}

func PromptSetupRerun(in io.Reader, out io.Writer, host string) (bool, error) {
	title := "You're already connected."
	host = strings.TrimSpace(host)
	if host != "" {
		title = "You're already connected to " + host + "."
	}
	return promptConfirm(in, out, title, "Set up a different server?")
}
