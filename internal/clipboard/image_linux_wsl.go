// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// isWSL matches the codex clipboard fallback behavior.
func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		version := strings.ToLower(string(data))
		if strings.Contains(version, "microsoft") || strings.Contains(version, "wsl") {
			return true
		}
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	return false
}

func readWSLClipboardPNG() ([]byte, error) {
	const script = `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $img = Get-Clipboard -Format Image; if ($img -ne $null) { $p=[System.IO.Path]::GetTempFileName(); $p = [System.IO.Path]::ChangeExtension($p,'png'); $img.Save($p,[System.Drawing.Imaging.ImageFormat]::Png); Write-Output $p } else { exit 1 }`
	for _, cmd := range []string{"powershell.exe", "pwsh", "powershell"} {
		out, err := exec.Command(cmd, "-NoProfile", "-Command", script).Output()
		if err != nil {
			continue
		}
		winPath := strings.TrimSpace(string(out))
		if winPath == "" {
			continue
		}
		wslPath, ok := windowsPathToWSL(winPath)
		if !ok {
			continue
		}
		data, err := os.ReadFile(wslPath)
		if err != nil {
			continue
		}
		_ = os.Remove(wslPath)
		return data, nil
	}
	return nil, ErrNoImage
}

func windowsPathToWSL(input string) (string, bool) {
	if strings.HasPrefix(input, "\\\\") {
		return "", false
	}
	if len(input) < 3 || input[1] != ':' {
		return "", false
	}
	drive := input[0]
	if drive >= 'A' && drive <= 'Z' {
		drive = drive + ('a' - 'A')
	}
	if drive < 'a' || drive > 'z' {
		return "", false
	}
	path := strings.ReplaceAll(input[2:], "\\", "/")
	path = strings.TrimLeft(path, "/")
	return fmt.Sprintf("/mnt/%c/%s", drive, path), true
}
