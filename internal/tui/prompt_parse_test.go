// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import "testing"

func TestParseYesNo(t *testing.T) {
	cases := []struct {
		input string
		want  bool
		ok    bool
	}{
		{input: "y", want: true, ok: true},
		{input: "yes", want: true, ok: true},
		{input: "n", want: false, ok: true},
		{input: "no", want: false, ok: true},
		{input: "", want: false, ok: true},
		{input: "maybe", want: false, ok: false},
	}
	for _, tc := range cases {
		got, ok := parseYesNo(tc.input)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("parseYesNo(%q) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseSelectionIndices(t *testing.T) {
	indices, err := parseSelectionIndices("1,3", 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Fatalf("unexpected indices: %v", indices)
	}

	indices, err = parseSelectionIndices("2", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(indices) != 1 || indices[0] != 1 {
		t.Fatalf("unexpected indices: %v", indices)
	}

	indices, err = parseSelectionIndices("", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(indices) != 0 {
		t.Fatalf("expected empty indices, got %v", indices)
	}

	if _, err = parseSelectionIndices("5", 3); err == nil {
		t.Fatalf("expected out of range error")
	}
}
