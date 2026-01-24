// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package model

import (
	"fmt"
	"os"
	"strings"

	"github.com/shayne/viberun/internal/ui/render"
	"github.com/shayne/viberun/internal/ui/styles"
)

type helpLine struct {
	cmd    string
	desc   string
	indent bool
}

func commandLinesForScope(scope shellScope, includeAdvanced bool) []helpLine {
	specs := commandSpecsForScope(scope)
	lines := make([]helpLine, 0, len(specs))
	for _, spec := range specs {
		if spec.Hidden {
			continue
		}
		if spec.Advanced && !includeAdvanced {
			continue
		}
		lines = append(lines, helpLine{cmd: spec.Display, desc: spec.Summary})
		for _, child := range spec.Children {
			lines = append(lines, helpLine{cmd: child.Cmd, desc: child.Desc, indent: true})
		}
	}
	return lines
}

func advancedCommandLinesForScope(scope shellScope) []helpLine {
	specs := commandSpecsForScope(scope)
	lines := make([]helpLine, 0, len(specs))
	for _, spec := range specs {
		if spec.Hidden {
			continue
		}
		if !spec.Advanced {
			continue
		}
		lines = append(lines, helpLine{cmd: spec.Display, desc: spec.Summary})
		for _, child := range spec.Children {
			lines = append(lines, helpLine{cmd: child.Cmd, desc: child.Desc, indent: true})
		}
	}
	return lines
}

func helpLinesToRender(lines []helpLine) []render.HelpLine {
	out := make([]render.HelpLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, render.HelpLine{Cmd: line.cmd, Desc: line.desc, Indent: line.indent})
	}
	return out
}

func stylesForRender() styles.Styles {
	if os.Getenv("NO_COLOR") != "" {
		return styles.Styles{}
	}
	termValue := os.Getenv("TERM")
	if termValue == "" || termValue == "dumb" {
		return styles.Styles{}
	}
	return styles.DefaultStyles()
}

func renderHelpGlobal(showAll bool) string {
	s := stylesForRender()
	if !showAll {
		return render.RenderHelpTable("Commands (use help --all for advanced):", helpLinesToRender(commandLinesForScope(scopeGlobal, false)), s)
	}
	base := helpLinesToRender(commandLinesForScope(scopeGlobal, false))
	advanced := helpLinesToRender(advancedCommandLinesForScope(scopeGlobal))
	rows := []string{""}
	rows = append(rows, render.RenderHelpSection("Commands:", base, s)...)
	if len(advanced) > 0 {
		rows = append(rows, "")
		rows = append(rows, render.RenderHelpSection("Advanced:", advanced, s)...)
	}
	rows = append(rows, "")
	rows = append(rows, "Run "+s.Value.Render("`help <command>`")+" for more details.")
	return strings.Join(rows, "\n")
}

func renderHelpApp() string {
	s := stylesForRender()
	return render.RenderHelpTable("Commands:", helpLinesToRender(commandLinesForScope(scopeAppConfig, false)), s)
}

func renderCommandHelp(name string, scope shellScope) string {
	spec, ok := lookupCommandSpec(scope, name)
	if !ok && scope == scopeAppConfig {
		spec, ok = lookupCommandSpec(scopeGlobal, name)
	}
	if !ok {
		return fmt.Sprintf("Unknown command: %s", name)
	}
	s := stylesForRender()
	labelStyle := s.HelpLabel
	usageStyle := s.HelpUsage
	examplesStyle := s.HelpExamples
	lines := []string{""}
	lines = append(lines, fmt.Sprintf("%s %s", labelStyle.Render("Command:"), spec.Display))
	if strings.TrimSpace(spec.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, spec.Description)
	}
	if strings.TrimSpace(spec.Usage) != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s %s", usageStyle.Render("Usage:"), spec.Usage))
	}
	if len(spec.Options) > 0 {
		lines = append(lines, "")
		lines = append(lines, usageStyle.Render("Options:"))
		for _, opt := range spec.Options {
			lines = append(lines, "  "+opt)
		}
	}
	if len(spec.Examples) > 0 {
		lines = append(lines, "")
		lines = append(lines, examplesStyle.Render("Examples:"))
		for _, ex := range spec.Examples {
			lines = append(lines, "  "+ex)
		}
	}
	return strings.Join(lines, "\n")
}

func renderSetupIntro() string {
	s := stylesForRender()
	example := s.Value.Render("root@1.2.3.4")
	lines := []render.InfoLine{
		{Label: "Step 1", Desc: "Choose a server (DigitalOcean, Hetzner, or a home server)."},
		{Label: "Step 2", Desc: "Make sure you can log in (username + IP or hostname)."},
		{Label: "Step 3", Desc: fmt.Sprintf("Example login: %s", example)},
	}
	header := "Setup: connect your server"
	return render.RenderInfoTable(header, lines, s)
}
