// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"errors"
	"strings"
)

type parsedCommand struct {
	name            string
	args            []string
	enforceExisting bool
}

func parseShellCommand(line string) (parsedCommand, error) {
	fields, err := splitShellArgs(line)
	if err != nil {
		return parsedCommand{}, err
	}
	if len(fields) == 0 {
		return parsedCommand{}, nil
	}
	rawName := fields[0]
	lowerName := strings.ToLower(rawName)
	name := lowerName
	if !isKnownCommandName(lowerName) {
		name = rawName
	}
	return parsedCommand{name: name, args: fields[1:]}, nil
}

func splitShellArgs(input string) ([]string, error) {
	var out []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		out = append(out, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == ' ' || r == '\t':
			if inSingle || inDouble {
				current.WriteRune(r)
			} else {
				flush()
			}
		default:
			current.WriteRune(r)
		}
	}
	if escaped || inSingle || inDouble {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return out, nil
}

func isKnownCommandName(name string) bool {
	name = normalizeCommandName(name)
	if name == "" {
		return false
	}
	if _, ok := lookupCommandSpec(scopeGlobal, name); ok {
		return true
	}
	if _, ok := lookupCommandSpec(scopeAppConfig, name); ok {
		return true
	}
	return false
}
