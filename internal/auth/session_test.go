// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSessionSignVerify(t *testing.T) {
	key := []byte("secret")
	session, err := NewSession("alice", time.Minute)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	token, err := SignSession(session, key)
	if err != nil {
		t.Fatalf("SignSession: %v", err)
	}
	if !strings.Contains(token, ".") {
		t.Fatalf("expected signed token")
	}
	verified, err := VerifySession(token, key)
	if err != nil {
		t.Fatalf("VerifySession: %v", err)
	}
	if verified.Username != "alice" {
		t.Fatalf("unexpected username: %q", verified.Username)
	}
}

func TestSessionExpired(t *testing.T) {
	key := []byte("secret")
	session := Session{
		Version:   1,
		Username:  "bob",
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
		Nonce:     "abc",
	}
	token, err := SignSession(session, key)
	if err != nil {
		t.Fatalf("SignSession: %v", err)
	}
	if _, err := VerifySession(token, key); err != ErrExpiredToken {
		t.Fatalf("expected expired token error, got %v", err)
	}
}
