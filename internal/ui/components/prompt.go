// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package components

import (
	"fmt"
	"strings"
)

func PromptPrefix(app string, inAppScope bool) string {
	if inAppScope && strings.TrimSpace(app) != "" {
		return fmt.Sprintf("viberun %s > ", app)
	}
	return "viberun > "
}

func PromptLine(app string, inAppScope bool, input string) string {
	return PromptPrefix(app, inAppScope) + input
}
