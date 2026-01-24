// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func queryPalette() Palette {
	palette := queryDefaultColors()
	if palette.HasBG || palette.HasFG {
		return palette
	}
	return paletteFromEnv()
}

func modeFromPalette(palette Palette) Mode {
	if palette.HasBG {
		if isLight(palette.BG) {
			return ModeLight
		}
		return ModeDark
	}
	return ModeUnknown
}

func isLight(color RGB) bool {
	luma := 0.2126*float64(color.R) + 0.7152*float64(color.G) + 0.0722*float64(color.B)
	return luma >= 128.0
}

func paletteFromEnv() Palette {
	value := strings.TrimSpace(os.Getenv("COLORFGBG"))
	if value == "" {
		return Palette{}
	}
	parts := strings.Split(value, ";")
	if len(parts) < 2 {
		return Palette{}
	}
	fg := parseEnvIndex(parts[0])
	bg := parseEnvIndex(parts[len(parts)-1])
	var palette Palette
	if fg >= 0 && fg < len(xterm16) {
		palette.FG = xterm16[fg]
		palette.HasFG = true
	}
	if bg >= 0 && bg < len(xterm16) {
		palette.BG = xterm16[bg]
		palette.HasBG = true
	}
	return palette
}

func parseEnvIndex(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	idx, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return idx
}

func parseOSCResponse(response string, code int) (RGB, bool) {
	start := strings.Index(response, "]")
	if start == -1 {
		return RGB{}, false
	}
	payload := response[start+1:]
	payload = strings.TrimSuffix(payload, "\a")
	payload = strings.TrimSuffix(payload, "\x1b\\")
	prefix := fmt.Sprintf("%d;", code)
	if !strings.HasPrefix(payload, prefix) {
		return RGB{}, false
	}
	payload = strings.TrimPrefix(payload, prefix)
	payload = strings.TrimPrefix(payload, "rgb:")
	parts := strings.Split(payload, "/")
	if len(parts) < 3 {
		return RGB{}, false
	}
	r, ok := parseChannel(parts[0])
	if !ok {
		return RGB{}, false
	}
	g, ok := parseChannel(parts[1])
	if !ok {
		return RGB{}, false
	}
	b, ok := parseChannel(parts[2])
	if !ok {
		return RGB{}, false
	}
	return RGB{R: r, G: g, B: b}, true
}

func parseChannel(value string) (uint8, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if len(value) > 4 {
		return 0, false
	}
	n, err := strconv.ParseUint(value, 16, 16)
	if err != nil {
		return 0, false
	}
	max := (1 << (len(value) * 4)) - 1
	if max <= 0 {
		return 0, false
	}
	if max != 255 {
		n = (n * 255) / uint64(max)
	}
	return uint8(n), true
}

var xterm16 = []RGB{
	{R: 0, G: 0, B: 0},       // 0 black
	{R: 128, G: 0, B: 0},     // 1 maroon
	{R: 0, G: 128, B: 0},     // 2 green
	{R: 128, G: 128, B: 0},   // 3 olive
	{R: 0, G: 0, B: 128},     // 4 navy
	{R: 128, G: 0, B: 128},   // 5 purple
	{R: 0, G: 128, B: 128},   // 6 teal
	{R: 192, G: 192, B: 192}, // 7 silver
	{R: 128, G: 128, B: 128}, // 8 grey
	{R: 255, G: 0, B: 0},     // 9 red
	{R: 0, G: 255, B: 0},     // 10 lime
	{R: 255, G: 255, B: 0},   // 11 yellow
	{R: 0, G: 0, B: 255},     // 12 blue
	{R: 255, G: 0, B: 255},   // 13 fuchsia
	{R: 0, G: 255, B: 255},   // 14 aqua
	{R: 255, G: 255, B: 255}, // 15 white
}
