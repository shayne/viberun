// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && clipboard_x11

package clipboard

import (
	"errors"
	"fmt"
	"sync"

	"golang.design/x/clipboard"
)

var initOnce sync.Once
var initErr error

// ReadImagePNG returns PNG-encoded bytes from the system clipboard.
// It returns ErrNoImage when no image data is available.
func ReadImagePNG() ([]byte, error) {
	if data, err := readClipboardPNG(); err == nil {
		return data, nil
	} else if !errors.Is(err, ErrNoImage) {
		return nil, err
	}

	if isWSL() {
		if data, err := readWSLClipboardPNG(); err == nil {
			return data, nil
		}
	}

	return nil, ErrNoImage
}

func readClipboardPNG() ([]byte, error) {
	initOnce.Do(func() {
		initErr = clipboard.Init()
	})
	if initErr != nil {
		return nil, fmt.Errorf("clipboard unavailable: %w", initErr)
	}
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
}
