// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts

// PasswordPrompt models a password entry prompt.
type PasswordPrompt struct{}

// NewPasswordPrompt constructs a password entry prompt flow.
func NewPasswordPrompt() PasswordPrompt {
	return PasswordPrompt{}
}
