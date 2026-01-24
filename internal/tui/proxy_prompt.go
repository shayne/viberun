// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/shayne/viberun/internal/proxy"
)

func PromptProxyDomain(in io.Reader, out io.Writer, prefix string) (string, error) {
	return promptProxyDomain(in, out, prefix, "")
}

func PromptProxyDomainWithDefault(in io.Reader, out io.Writer, prefix string, defaultDomain string) (string, error) {
	return promptProxyDomain(in, out, prefix, defaultDomain)
}

func promptProxyDomain(in io.Reader, out io.Writer, prefix string, defaultDomain string) (string, error) {
	value := strings.TrimSpace(defaultDomain)
	prompt := strings.TrimSpace(prefix)
	if prompt == "" {
		prompt = "myapp."
	}
	promptStyle := lipgloss.NewStyle().Faint(true)
	promptText := promptStyle.Render(prompt)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Public domain name").
				Description(fmt.Sprintf("Your apps will be available at %s<domain>", prompt)).
				Prompt(promptText).
				Placeholder("mydomain.com").
				Value(&value).
				Validate(func(input string) error {
					_, err := proxy.NormalizeDomainSuffix(input)
					return err
				}),
		),
	)

	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))
	if err := form.Run(); err != nil {
		return "", err
	}
	return proxy.NormalizeDomainSuffix(value)
}
