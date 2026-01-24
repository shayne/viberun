// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/shayne/viberun/internal/proxy"
)

func PromptProxyDomain(in io.Reader, out io.Writer, prefix string) (string, error) {
	return promptProxyDomain(in, out, prefix, "")
}

func PromptProxyDomainWithDefault(in io.Reader, out io.Writer, prefix string, defaultDomain string) (string, error) {
	return promptProxyDomain(in, out, prefix, defaultDomain)
}

func promptProxyDomain(in io.Reader, out io.Writer, prefix string, defaultDomain string) (string, error) {
	prompt := strings.TrimSpace(prefix)
	if prompt == "" {
		prompt = "myapp."
	}
	input, err := promptInput(in, out, "Public domain name", fmt.Sprintf("Your apps will be available at %s<domain>", prompt), "mydomain.com", func(input string) error {
		_, err := proxy.NormalizeDomainSuffix(input)
		return err
	})
	if err != nil {
		return "", err
	}
	return proxy.NormalizeDomainSuffix(input)
}
