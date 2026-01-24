// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"strings"
)

func PromptWipeConfirm(in io.Reader, out io.Writer) error {
	value, err := promptInput(in, out, "Type WIPE to confirm",
		"This permanently removes all viberun data from the server. This cannot be undone.",
		"WIPE",
		func(input string) error {
			if strings.TrimSpace(input) != "WIPE" {
				return errors.New("type WIPE to confirm")
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) != "WIPE" {
		return errors.New("confirmation did not match")
	}
	return nil
}

func PromptWipeDecision(in io.Reader, out io.Writer, host string) (bool, error) {
	host = strings.TrimSpace(host)
	title := "Wipe this server?"
	if host != "" {
		title = "Wipe " + host + "?"
	}
	return promptConfirm(in, out, title, "This removes all viberun data, containers, and configuration from that server.")
}
