// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Progress struct {
	out     io.Writer
	enabled bool
	action  string
	host    string

	plain *plainProgress
	color Colorizer

	mu        sync.Mutex
	stopped   bool
	suspended bool
	current   string
	spinner   *Spinner
}

func NewProgress(out io.Writer, enabled bool, action, host string) *Progress {
	return &Progress{
		out:     out,
		enabled: enabled,
		action:  strings.TrimSpace(action),
		host:    strings.TrimSpace(host),
		plain:   newPlainProgress(out, action, host),
		color:   NewColorizer(enabled),
	}
}

func (p *Progress) Start() {
	if p.enabled {
		p.printHeader()
		return
	}
	p.plain.Header()
}

func (p *Progress) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	p.stopSpinner(false)
	p.plain.MarkHeaderDone()
}

func (p *Progress) Suspend() {
	p.mu.Lock()
	p.suspended = true
	p.mu.Unlock()
	p.stopSpinner(true)
}

func (p *Progress) Resume() {
	p.mu.Lock()
	p.suspended = false
	p.mu.Unlock()
}

func (p *Progress) Step(name string) {
	p.mu.Lock()
	if p.current == name {
		p.mu.Unlock()
		return
	}
	if p.suspended {
		p.mu.Unlock()
		return
	}
	p.current = name
	p.mu.Unlock()

	if p.enabled {
		p.stopSpinner(true)
		p.spinner = p.newSpinner(name)
		return
	}
	p.plain.StartStep(name)
}

func (p *Progress) Update(detail string) {
	p.mu.Lock()
	name := p.current
	sp := p.spinner
	suspended := p.suspended
	p.mu.Unlock()

	if !p.enabled || suspended || sp == nil {
		return
	}
	text := name
	if detail != "" {
		text = fmt.Sprintf("%s %s", name, detail)
	}
	sp.Update(text)
}

func (p *Progress) Done(detail string) {
	p.mu.Lock()
	name := p.current
	p.current = ""
	p.mu.Unlock()

	if name == "" {
		return
	}
	p.stopSpinner(true)
	if p.enabled {
		p.printStatus("OK", name, detail)
		return
	}
	p.plain.DoneStep(detail)
}

func (p *Progress) Fail(detail string) {
	p.mu.Lock()
	name := p.current
	p.current = ""
	p.mu.Unlock()

	if name == "" {
		return
	}
	p.stopSpinner(true)
	if p.enabled {
		p.printStatus("ERR", name, detail)
		return
	}
	p.plain.FailStep(detail)
}

func (p *Progress) Info(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	p.stopSpinner(true)
	if p.enabled {
		fmt.Fprintln(p.out, msg)
		return
	}
	p.plain.Info(msg)
}

func (p *Progress) Warn(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	p.stopSpinner(true)
	if p.enabled {
		fmt.Fprintln(p.out, p.color.Wrap(ColorYellow, msg))
		return
	}
	p.plain.Warn(msg)
}

func (p *Progress) printHeader() {
	label := strings.TrimSpace(p.action)
	if label == "" {
		label = "viberun"
	} else {
		label = fmt.Sprintf("viberun %s", label)
	}
	if p.host != "" {
		label = fmt.Sprintf("%s (host=%s)", label, p.host)
	}
	fmt.Fprintf(p.out, "[+] %s\n", label)
}

func (p *Progress) newSpinner(text string) *Spinner {
	sp := NewSpinner(p.out,
		WithFrames(DefaultFrames),
		WithInterval(120*time.Millisecond),
		WithColor(p.color, ColorYellow),
		WithHideCursor(true),
	)
	sp.Start(text)
	return sp
}

func (p *Progress) stopSpinner(clear bool) {
	p.mu.Lock()
	sp := p.spinner
	p.spinner = nil
	p.mu.Unlock()

	if sp == nil {
		return
	}
	sp.Stop(clear)
}

func (p *Progress) printStatus(status, name, detail string) {
	label := status
	switch status {
	case "OK":
		label = p.color.Wrap(ColorGreen, "✔")
	case "ERR":
		label = p.color.Wrap(ColorRed, "✖")
	}
	line := fmt.Sprintf("%s %s", label, name)
	if detail != "" {
		line = fmt.Sprintf("%s (%s)", line, detail)
	}
	fmt.Fprintln(p.out, line)
}

type plainProgress struct {
	out        io.Writer
	action     string
	host       string
	headerDone bool
	current    string
}

func newPlainProgress(out io.Writer, action, host string) *plainProgress {
	return &plainProgress{out: out, action: action, host: host}
}

func (p *plainProgress) Header() {
	p.headerDone = true
}

func (p *plainProgress) MarkHeaderDone() {
	p.headerDone = true
}

func (p *plainProgress) Info(label string) {
	p.Header()
	fmt.Fprintln(p.out, p.line("info", "", label))
}

func (p *plainProgress) Warn(label string) {
	p.Header()
	fmt.Fprintln(p.out, p.line("warn", "", label))
}

func (p *plainProgress) StartStep(name string) {
	p.Header()
	p.current = name
	fmt.Fprintln(p.out, p.line("running", name, ""))
}

func (p *plainProgress) DoneStep(detail string) {
	p.Header()
	if p.current == "" {
		return
	}
	fmt.Fprintln(p.out, p.line("ok", p.current, detail))
	p.current = ""
}

func (p *plainProgress) FailStep(detail string) {
	p.Header()
	if p.current == "" {
		return
	}
	fmt.Fprintln(p.out, p.line("err", p.current, detail))
	p.current = ""
}

func (p *plainProgress) line(status, step, detail string) string {
	parts := []string{
		"action", p.action,
		"host", p.host,
		"status", status,
	}
	if step != "" {
		parts = append(parts, "step", step)
	}
	if detail != "" {
		parts = append(parts, "detail", detail)
	}
	return formatProgressKV(parts...)
}

func formatProgressKV(parts ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(parts); i += 2 {
		key := strings.TrimSpace(parts[i])
		val := strings.TrimSpace(parts[i+1])
		if key == "" || val == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(quoteProgressKV(val))
	}
	return b.String()
}

func quoteProgressKV(val string) string {
	if progressNeedsQuote(val) {
		return strconv.Quote(val)
	}
	return val
}

func progressNeedsQuote(val string) bool {
	for _, r := range val {
		switch r {
		case ' ', '\t', '\n', '"', '=':
			return true
		}
	}
	return false
}
