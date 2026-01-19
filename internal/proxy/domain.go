// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import (
	"fmt"
	"strings"
)

func NormalizeDomainSuffix(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", fmt.Errorf("domain is required")
	}
	if strings.Contains(trimmed, "://") {
		return "", fmt.Errorf("domain must not include a scheme")
	}
	if strings.ContainsAny(trimmed, " /\t\n\r") {
		return "", fmt.Errorf("domain must not contain spaces")
	}
	if strings.ContainsAny(trimmed, "/:@") {
		return "", fmt.Errorf("domain must not include a path or port")
	}
	if strings.HasPrefix(trimmed, ".") || strings.HasSuffix(trimmed, ".") {
		return "", fmt.Errorf("domain must not start or end with a dot")
	}
	if strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("domain must not contain empty labels")
	}
	if !strings.Contains(trimmed, ".") {
		return "", fmt.Errorf("domain must contain a dot")
	}
	if len(trimmed) > 253 {
		return "", fmt.Errorf("domain is too long")
	}
	labels := strings.Split(trimmed, ".")
	for _, label := range labels {
		if label == "" {
			return "", fmt.Errorf("domain must not contain empty labels")
		}
		if len(label) > 63 {
			return "", fmt.Errorf("domain label %q is too long", label)
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("domain label %q must not start or end with '-'", label)
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-':
			default:
				return "", fmt.Errorf("domain label %q has invalid character %q", label, r)
			}
		}
	}
	return trimmed, nil
}

func NormalizeAppName(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", fmt.Errorf("app name is required")
	}
	if len(trimmed) > 63 {
		return "", fmt.Errorf("app name is too long")
	}
	if strings.HasPrefix(trimmed, "-") || strings.HasSuffix(trimmed, "-") {
		return "", fmt.Errorf("app name must not start or end with '-'")
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return "", fmt.Errorf("app name has invalid character %q", r)
		}
	}
	return trimmed, nil
}
