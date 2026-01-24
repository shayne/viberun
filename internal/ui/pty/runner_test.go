// Copyright (c) 2026 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pty

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"

	"github.com/shayne/viberun/internal/ui/model"
)

func TestShellHelpPTY(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")

	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	if err := pty.Setsize(slave, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("set pty size: %v", err)
	}
	m := model.NewShellModel()
	program := tea.NewProgram(m, tea.WithInput(slave), tea.WithOutput(slave), tea.WithoutRenderer())

	type result struct {
		model tea.Model
		err   error
	}
	done := make(chan result, 1)
	go func() {
		finalModel, err := program.Run()
		done <- result{model: finalModel, err: err}
	}()

	program.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, r := range "help" {
		program.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	program.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	time.Sleep(100 * time.Millisecond)

	wantPath := filepath.Join("..", "testdata", "help_global.txt")
	wantBytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	want := strings.TrimSpace(string(wantBytes))

	program.Quit()
	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("program error: %v", res.err)
		}
		shell, ok := res.model.(model.ShellModel)
		if !ok {
			t.Fatalf("unexpected model type %T", res.model)
		}
		view := fmt.Sprint(shell.View().Content)
		if !strings.Contains(view, want) {
			t.Fatalf("output missing help snapshot:\n%s", view)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("program did not exit")
	}
}
