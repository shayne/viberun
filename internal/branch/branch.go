// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package branch

import (
	"fmt"

	"github.com/shayne/viberun/internal/proxy"
)

const maxAppLength = 63

func NormalizeBranchName(raw string) (string, error) {
	return proxy.NormalizeAppName(raw)
}

func DerivedAppName(base string, branch string) (string, error) {
	baseNorm, err := proxy.NormalizeAppName(base)
	if err != nil {
		return "", err
	}
	branchNorm, err := NormalizeBranchName(branch)
	if err != nil {
		return "", err
	}
	derived := baseNorm + "--" + branchNorm
	if len(derived) > maxAppLength {
		return "", fmt.Errorf("branch app name is too long")
	}
	return derived, nil
}
