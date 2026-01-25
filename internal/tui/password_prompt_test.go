// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptPassword_NonTTYFallback(t *testing.T) {
	in := strings.NewReader("secret\n")
	var out bytes.Buffer
	got, err := PromptPassword(in, &out, "Password")
	if err != nil || got != "secret" {
		t.Fatalf("unexpected result: %q err=%v", got, err)
	}
}
