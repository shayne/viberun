// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix
// +build !unix

package theme

func queryDefaultColors() Palette {
	return Palette{}
}
