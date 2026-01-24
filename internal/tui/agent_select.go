// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shayne/viberun/internal/agents"
)

func SelectDefaultAgent(in io.Reader, out io.Writer) (string, error) {
	definitions, err := agents.Builtins()
	if err != nil {
		return "", err
	}

	options := make([]SelectOption, 0, len(definitions)+2)
	for _, def := range definitions {
		label := strings.TrimSpace(def.Label)
		if label == "" {
			label = def.ID
		}
		options = append(options, SelectOption{Label: label, Value: def.ID})
	}
	options = append(options,
		SelectOption{Label: "Custom (npx)", Value: "custom:npx"},
		SelectOption{Label: "Custom (uvx)", Value: "custom:uvx"},
	)

	choice := agents.DefaultProvider()
	selected, err := PromptSelect(in, out, "Choose your default agent", "", options, choice)
	if err != nil {
		return "", err
	}
	choice = strings.TrimSpace(selected)
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
	value, err := promptInput(in, out, fmt.Sprintf("%s package", runner), "", "", func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New("package is required")
		}
		if strings.ContainsAny(value, " \t") {
			return errors.New("package must not contain spaces")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("package is required")
	}
	return fmt.Sprintf("%s:%s", runner, value), nil
}
