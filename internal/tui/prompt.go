// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/shayne/viberun/internal/tui/dialogs"
)

type SelectOption struct {
	Label string
	Value string
}

func promptInput(in io.Reader, out io.Writer, title, description, placeholder string, validate func(string) error) (string, error) {
	return promptInputWithDefault(in, out, title, description, placeholder, "", validate)
}

func promptInputWithDefault(in io.Reader, out io.Writer, title, description, placeholder, defaultValue string, validate func(string) error) (string, error) {
	if useDialogPrompts(in, out) {
		return promptInputDialog(in, out, title, description, placeholder, defaultValue, validate, false)
	}
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
	if useDialogPrompts(in, out) {
		dialog := dialogs.NewConfirmDialog("confirm", title, description, false)
		result, err := dialogs.Run(in, out, dialog)
		if err != nil {
			return false, err
		}
		if result == nil || result.Cancelled {
			return false, nil
		}
		return result.Confirmed, nil
	}
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
	if useDialogPrompts(in, out) {
		dialogOptions := make([]dialogs.Option, 0, len(options))
		for _, opt := range options {
			dialogOptions = append(dialogOptions, dialogs.Option{Label: opt.Label, Value: opt.Value})
		}
		dialog := dialogs.NewSelectDialog("select", title, description, dialogOptions, defaultValue)
		result, err := dialogs.Run(in, out, dialog)
		if err != nil {
			return "", err
		}
		if result == nil || result.Cancelled {
			return defaultValue, nil
		}
		return result.Choice, nil
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
	if useDialogPrompts(in, out) {
		dialogOptions := make([]dialogs.Option, 0, len(options))
		for _, opt := range options {
			dialogOptions = append(dialogOptions, dialogs.Option{Label: opt.Label, Value: opt.Value})
		}
		dialog := dialogs.NewMultiSelectDialog("multi-select", title, description, dialogOptions, selected)
		result, err := dialogs.Run(in, out, dialog)
		if err != nil {
			return nil, err
		}
		if result == nil || result.Cancelled {
			return selected, nil
		}
		return result.Choices, nil
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

func promptInputDialog(in io.Reader, out io.Writer, title, description, placeholder, defaultValue string, validate func(string) error, secret bool) (string, error) {
	field := dialogs.Field{
		ID:          "value",
		Title:       title,
		Description: description,
		Placeholder: placeholder,
		Required:    validate != nil,
		Default:     defaultValue,
		Secret:      secret,
		Validate:    validate,
	}
	dialog := dialogs.NewFormDialog("prompt", title, description, []dialogs.Field{field})
	result, err := dialogs.Run(in, out, dialog)
	if err != nil {
		return "", err
	}
	if result == nil || result.Cancelled {
		return "", errors.New("prompt cancelled")
	}
	return strings.TrimSpace(result.Values["value"]), nil
}

func useDialogPrompts(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(inFile.Fd())) {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok || !term.IsTerminal(int(outFile.Fd())) {
		return false
	}
	return true
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
