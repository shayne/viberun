// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package branch

import "testing"

func TestDerivedAppName(t *testing.T) {
	got, err := DerivedAppName("company", "contact-form")
	if err != nil {
		t.Fatalf("DerivedAppName error: %v", err)
	}
	if got != "company--contact-form" {
		t.Fatalf("got %q", got)
	}
}

func TestDerivedAppNameRejectsLong(t *testing.T) {
	_, err := DerivedAppName("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "b")
	if err == nil {
		t.Fatalf("expected error for too-long derived name")
	}
}

func TestNormalizeBranchName(t *testing.T) {
	got, err := NormalizeBranchName("Contact-Form")
	if err != nil {
		t.Fatalf("NormalizeBranchName error: %v", err)
	}
	if got != "contact-form" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeBranchNameRejectsInvalid(t *testing.T) {
	_, err := NormalizeBranchName("bad/branch")
	if err == nil {
		t.Fatalf("expected error for invalid branch")
	}
}
