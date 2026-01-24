// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts

// WipePrompt models the two-step wipe confirmation flow.
type WipePrompt struct{}

// NewWipePrompt constructs a new wipe prompt flow.
func NewWipePrompt() WipePrompt {
	return WipePrompt{}
}

// RequiresConfirm returns true if the prompt needs an explicit yes/no step.
func (WipePrompt) RequiresConfirm() bool {
	return true
}

// RequiresToken returns true if the prompt needs the WIPE token.
func (WipePrompt) RequiresToken() bool {
	return true
}
