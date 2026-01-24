// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix
// +build !unix

package theme

import "time"

// SetOSCReadHook is a no-op on non-UNIX platforms.
func SetOSCReadHook(h func(code int, timeout time.Duration) (string, error)) {
	_ = h
}

func queryDefaultColors() Palette {
	return Palette{}
}
