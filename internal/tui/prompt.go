// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

type SelectOption struct {
	Label string
	Value string
}

func promptInput(in io.Reader, out io.Writer, title, description, placeholder string, validate func(string) error) (string, error) {
	reader := bufio.NewReader(in)
	printPromptHeader(out, title, description, placeholder)
	for {
		fmt.Fprint(out, "> ")
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if validate != nil {
			if err := validate(line); err != nil {
				fmt.Fprintln(out, err.Error())
				continue
			}
		}
		return strings.TrimSpace(line), nil
	}
}

func promptConfirm(in io.Reader, out io.Writer, title, description string) (bool, error) {
	reader := bufio.NewReader(in)
	printPromptHeader(out, title, description, "")
	for {
		fmt.Fprint(out, "Confirm [y/N]: ")
		line, err := readLine(reader)
		if err != nil {
			return false, err
		}
		value, ok := parseYesNo(line)
		if !ok {
			fmt.Fprintln(out, "Please enter y or n.")
			continue
		}
		return value, nil
	}
}

func PromptSelect(in io.Reader, out io.Writer, title, description string, options []SelectOption, defaultValue string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options available")
	}
	reader := bufio.NewReader(in)
	printPromptHeader(out, title, description, "")
	for i, opt := range options {
		label := opt.Label
		if label == "" {
			label = opt.Value
		}
		marker := " "
		if opt.Value == defaultValue {
			marker = "*"
		}
		fmt.Fprintf(out, "%s %d) %s\n", marker, i+1, label)
	}
	for {
		fmt.Fprint(out, "Select option: ")
		line, err := readLine(reader)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) == "" && defaultValue != "" {
			return defaultValue, nil
		}
		indices, err := parseSelectionIndices(line, len(options))
		if err != nil || len(indices) != 1 {
			fmt.Fprintln(out, "Please select one option by number.")
			continue
		}
		return options[indices[0]].Value, nil
	}
}

func PromptMultiSelect(in io.Reader, out io.Writer, title, description string, options []SelectOption, selected []string) ([]string, error) {
	if len(options) == 0 {
		return nil, errors.New("no options available")
	}
	selectedSet := map[string]bool{}
	for _, value := range selected {
		selectedSet[value] = true
	}
	reader := bufio.NewReader(in)
	printPromptHeader(out, title, description, "")
	for i, opt := range options {
		label := opt.Label
		if label == "" {
			label = opt.Value
		}
		marker := " "
		if selectedSet[opt.Value] {
			marker = "x"
		}
		fmt.Fprintf(out, "[%s] %d) %s\n", marker, i+1, label)
	}
	for {
		fmt.Fprint(out, "Select options (comma-separated, blank to keep current): ")
		line, err := readLine(reader)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			return selected, nil
		}
		indices, err := parseSelectionIndices(line, len(options))
		if err != nil {
			fmt.Fprintln(out, "Please select valid option numbers.")
			continue
		}
		next := make([]string, 0, len(indices))
		for _, idx := range indices {
			next = append(next, options[idx].Value)
		}
		return next, nil
	}
}

func printPromptHeader(out io.Writer, title, description, placeholder string) {
	if strings.TrimSpace(title) != "" {
		fmt.Fprintln(out, title)
	}
	if strings.TrimSpace(description) != "" {
		fmt.Fprintln(out, description)
	}
	if strings.TrimSpace(placeholder) != "" {
		fmt.Fprintln(out, placeholder)
	}
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) {
		return strings.TrimRight(line, "\r\n"), nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
