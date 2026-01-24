// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"io"
	"strings"
)

func PromptProxyAuth(in io.Reader, out io.Writer, defaultUser string) (string, string, error) {
	username, err := promptInput(in, out, "Primary username", "", "", func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New("username is required")
		}
		if strings.ContainsAny(value, " \t") {
			return errors.New("username must not contain spaces")
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	password, err := PromptPassword(in, out, "Password")
	if err != nil {
		return "", "", err
	}
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return "", "", errors.New("username and password are required")
	}
	return username, password, nil
}
