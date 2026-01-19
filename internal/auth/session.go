// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid session token")
	ErrExpiredToken = errors.New("session token expired")
)

type Session struct {
	Version   int    `json:"v"`
	Username  string `json:"u"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"n"`
}

func NewSession(username string, ttl time.Duration) (Session, error) {
	if strings.TrimSpace(username) == "" {
		return Session{}, fmt.Errorf("username is required")
	}
	if ttl <= 0 {
		return Session{}, fmt.Errorf("ttl must be positive")
	}
	now := time.Now().UTC()
	nonce, err := randomNonce(16)
	if err != nil {
		return Session{}, err
	}
	return Session{
		Version:   1,
		Username:  username,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Nonce:     nonce,
	}, nil
}

func SignSession(session Session, key []byte) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("signing key is required")
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(encoded, key)
	return encoded + "." + sig, nil
}

func VerifySession(token string, key []byte) (Session, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Session{}, ErrInvalidToken
	}
	payload := strings.TrimSpace(parts[0])
	sig := strings.TrimSpace(parts[1])
	if payload == "" || sig == "" {
		return Session{}, ErrInvalidToken
	}
	if !verify(payload, sig, key) {
		return Session{}, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, ErrInvalidToken
	}
	if session.ExpiresAt <= time.Now().UTC().Unix() {
		return Session{}, ErrExpiredToken
	}
	if strings.TrimSpace(session.Username) == "" {
		return Session{}, ErrInvalidToken
	}
	return session, nil
}

func sign(payload string, key []byte) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func verify(payload string, signature string, key []byte) bool {
	if len(key) == 0 {
		return false
	}
	expected := sign(payload, key)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func randomNonce(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
