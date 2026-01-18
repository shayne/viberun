// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import "testing"

func TestProgressNeedsQuote(t *testing.T) {
	cases := map[string]bool{
		"plain":       false,
		"with space":  true,
		"tab\tvalue":  true,
		"line\nbreak": true,
		"quote\"here": true,
		"a=b":         true,
	}
	for input, expected := range cases {
		if got := progressNeedsQuote(input); got != expected {
			t.Fatalf("progressNeedsQuote(%q)=%v want %v", input, got, expected)
		}
	}
}

func TestQuoteProgressKV(t *testing.T) {
	if got := quoteProgressKV("plain"); got != "plain" {
		t.Fatalf("unexpected quote: %q", got)
	}
	if got := quoteProgressKV("has space"); got == "has space" {
		t.Fatalf("expected quoted value, got %q", got)
	}
}

func TestFormatProgressKV(t *testing.T) {
	got := formatProgressKV("action", "bootstrap", "detail", "hello world", "", "skip")
	want := "action=bootstrap detail=\"hello world\""
	if got != want {
		t.Fatalf("formatProgressKV=%q want %q", got, want)
	}
}
