// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSafeRedirectRelative(t *testing.T) {
	r := httptest.NewRequest("GET", "https://example.com/__viberun/auth/login", nil)
	got := SafeRedirect(r, "/dashboard?tab=1")
	if got != "/dashboard?tab=1" {
		t.Fatalf("unexpected redirect: %q", got)
	}
}

func TestSafeRedirectRejectsExternal(t *testing.T) {
	r := httptest.NewRequest("GET", "https://example.com/__viberun/auth/login", nil)
	got := SafeRedirect(r, "https://evil.example.net/steal")
	if got != "/" {
		t.Fatalf("expected fallback redirect, got %q", got)
	}
}
