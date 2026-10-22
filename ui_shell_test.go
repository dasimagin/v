//go:build unix

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUIShellRun(t *testing.T) {
	var mu sync.Mutex
	var out strings.Builder
	done := make(chan uiEvent, 1)
	s := newUIShell(func(e uiEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Kind {
		case "shell-out":
			out.WriteString(e.Text)
		case "shell-done":
			done <- e
		}
	})
	if err := s.Run(t.TempDir(), "a", "printf 'héllo\\n'; echo err >&2; exit 3", 0); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-done:
		if e.Turns != 3 {
			t.Errorf("exit code = %d, want 3", e.Turns)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no shell-done")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "héllo") || !strings.Contains(out.String(), "err") {
		t.Errorf("output = %q", out.String())
	}
}

func TestUIShellStop(t *testing.T) {
	done := make(chan uiEvent, 1)
	s := newUIShell(func(e uiEvent) {
		if e.Kind == "shell-done" {
			done <- e
		}
	})
	if err := s.Run(t.TempDir(), "b", "sleep 30", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	s.Stop("b")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the command was not stopped")
	}
}

func TestCleanOutput(t *testing.T) {
	if got := cleanOutput("\x1b[31mred\x1b[0m\r\nok\r50%"); got != "red\nok\n50%" {
		t.Errorf("cleanOutput = %q", got)
	}
}

func TestUIShellColumns(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 12; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%02d.txt", i)), nil, 0o644)
	}
	run := func(command string) string {
		var mu sync.Mutex
		var out strings.Builder
		done := make(chan struct{})
		s := newUIShell(func(e uiEvent) {
			mu.Lock()
			defer mu.Unlock()
			switch e.Kind {
			case "shell-out":
				out.WriteString(e.Text)
			case "shell-done":
				close(done)
			}
		})
		if err := s.Run(dir, "c", command, 0); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("no shell-done")
		}
		mu.Lock()
		defer mu.Unlock()
		return strings.TrimSpace(out.String())
	}
	if lines := strings.Count(run("ls"), "\n") + 1; lines >= 12 {
		t.Errorf("ls printed %d lines for 12 files: not in columns", lines)
	}
	if lines := strings.Count(run("ls | cat"), "\n") + 1; lines != 12 {
		t.Errorf("ls | cat printed %d lines, want 12", lines)
	}
}

func TestUIShellWidth(t *testing.T) {
	done := make(chan struct{})
	var mu sync.Mutex
	var out strings.Builder
	s := newUIShell(func(e uiEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Kind {
		case "shell-out":
			out.WriteString(e.Text)
		case "shell-done":
			close(done)
		}
	})
	if err := s.Run(t.TempDir(), "w", "echo $COLUMNS; stty size <&1", 137); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("no shell-done")
	}
	mu.Lock()
	defer mu.Unlock()
	if got := out.String(); !strings.Contains(got, "137\n") || !strings.Contains(got, "24 137") {
		t.Errorf("width not passed on: %q", got)
	}
}
