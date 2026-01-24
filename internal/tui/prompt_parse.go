// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"errors"
	"strconv"
	"strings"
)

func parseYesNo(input string) (bool, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(input))
	switch trimmed {
	case "", "n", "no":
		return false, true
	case "y", "yes":
		return true, true
	default:
		return false, false
	}
}

func parseSelectionIndices(input string, max int) ([]int, error) {
	if max <= 0 {
		return nil, errors.New("no options available")
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, ",")
	seen := map[int]bool{}
	indices := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, errors.New("invalid selection")
		}
		if value < 1 || value > max {
			return nil, errors.New("selection out of range")
		}
		idx := value - 1
		if seen[idx] {
			continue
		}
		seen[idx] = true
		indices = append(indices, idx)
	}
	return indices, nil
}
