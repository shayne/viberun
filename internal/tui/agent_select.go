// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/shayne/viberun/internal/agents"
)

func SelectDefaultAgent(in io.Reader, out io.Writer) (string, error) {
	definitions, err := agents.Builtins()
	if err != nil {
		return "", err
	}

	options := make([]huh.Option[string], 0, len(definitions)+2)
	for _, def := range definitions {
		label := strings.TrimSpace(def.Label)
		if label == "" {
			label = def.ID
		}
		options = append(options, huh.NewOption(label, def.ID))
	}
	options = append(options,
		huh.NewOption("Custom (npx)", "custom:npx"),
		huh.NewOption("Custom (uvx)", "custom:uvx"),
	)

	choice := agents.DefaultProvider()
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose your default agent").
				Options(options...).
				Value(&choice),
		),
	)

	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))

	if err := form.Run(); err != nil {
		return "", err
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return "", errors.New("no agent selected")
	}

	switch choice {
	case "custom:npx":
		return promptCustomRunner(in, out, "npx")
	case "custom:uvx":
		return promptCustomRunner(in, out, "uvx")
	default:
		return choice, nil
	}
}

func promptCustomRunner(in io.Reader, out io.Writer, runner string) (string, error) {
	var pkg string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("%s package", runner)).
				Prompt("> ").
				Value(&pkg).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("package is required")
					}
					if strings.ContainsAny(value, " \t") {
						return errors.New("package must not contain spaces")
					}
					return nil
				}),
		),
	)
	form.WithInput(in).WithOutput(out).WithTheme(promptTheme(out))
	if err := form.Run(); err != nil {
		return "", err
	}
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "", errors.New("package is required")
	}
	return fmt.Sprintf("%s:%s", runner, pkg), nil
}
