// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && !clipboard_x11

package clipboard

// ReadImagePNG returns PNG-encoded bytes from the system clipboard.
// It returns ErrNoImage when no image data is available.
func ReadImagePNG() ([]byte, error) {
	if isWSL() {
		if data, err := readWSLClipboardPNG(); err == nil {
			return data, nil
		}
	}
	return nil, ErrNoImage
}
