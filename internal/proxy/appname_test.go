// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import "testing"

func TestNormalizeAppName(t *testing.T) {
	cases := map[string]string{
		"myapp":     "myapp",
		"My-App":    "my-app",
		"app123":    "app123",
		"app-name":  "app-name",
		" appname ": "appname",
	}
	for input, expected := range cases {
		got, err := NormalizeAppName(input)
		if err != nil {
			t.Fatalf("NormalizeAppName(%q) error: %v", input, err)
		}
		if got != expected {
			t.Fatalf("NormalizeAppName(%q)=%q want %q", input, got, expected)
		}
	}
}

func TestNormalizeAppNameInvalid(t *testing.T) {
	cases := []string{
		"",
		" ",
		".",
		"my.app",
		"my_app",
		"my app",
		"-bad",
		"bad-",
		"UPPER.CASE",
		"app@host",
		"app:8080",
	}
	for _, input := range cases {
		if _, err := NormalizeAppName(input); err == nil {
			t.Fatalf("NormalizeAppName(%q) expected error", input)
		}
	}
}
