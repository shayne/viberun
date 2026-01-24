// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

type shellScope int

const (
	scopeGlobal shellScope = iota
	scopeAppConfig
)
