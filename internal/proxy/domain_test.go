// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy

import "testing"

func TestNormalizeDomainSuffix(t *testing.T) {
	valid := []string{
		"example.com",
		"my-domain.co.uk",
		"xn--example-2na.com",
		"EXAMPLE.COM",
	}
	for _, input := range valid {
		got, err := NormalizeDomainSuffix(input)
		if err != nil {
			t.Fatalf("NormalizeDomainSuffix(%q) error: %v", input, err)
		}
		if got == "" {
			t.Fatalf("NormalizeDomainSuffix(%q) empty", input)
		}
	}

	invalid := []string{
		"",
		"   ",
		"example",
		"http://example.com",
		"example.com/path",
		"example.com:8080",
		"exa_mple.com",
		"-bad.example.com",
		"bad-.example.com",
		".example.com",
		"example.com.",
		"example..com",
	}
	for _, input := range invalid {
		if _, err := NormalizeDomainSuffix(input); err == nil {
			t.Fatalf("NormalizeDomainSuffix(%q) expected error", input)
		}
	}
}
