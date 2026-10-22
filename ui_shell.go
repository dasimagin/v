//go:build unix

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/creack/pty"
)

const (
	uiShellMax  = 256 << 10 // bytes of output shown for one command
	uiShellCols = 100       // the width programs see when the page does not say, so that ls and friends lay out in columns
)

// uiShell runs the commands typed in the omnibox's shell mode and reports their
// output as events: shell-out (a chunk of text) and shell-done (the exit code).
type uiShell struct {
	emit func(uiEvent)

	mu   sync.Mutex
	runs map[string]*exec.Cmd
}

func newUIShell(emit func(uiEvent)) *uiShell {
	return &uiShell{emit: emit, runs: map[string]*exec.Cmd{}}
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07]*\x07|\x1b[()][A-Z0-9]")

// cleanOutput drops terminal escapes; a lone carriage return (a progress bar
// redrawing itself) becomes a line break.
func cleanOutput(s string) string {
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// chunkWriter turns what a command writes into events, never splitting a UTF-8 character.
type chunkWriter struct {
	emit func(uiEvent)
	id   string
	rest []byte
	sent int
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	b := append(w.rest, p...)
	n := len(b)
	// Hold back an incomplete UTF-8 character at the end until the rest arrives.
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				n = i
			}
			break
		}
	}
	w.rest = append([]byte(nil), b[n:]...)
	text := cleanOutput(string(b[:n]))
	if text == "" {
		return len(p), nil
	}
	switch {
	case w.sent >= uiShellMax:
		return len(p), nil
	case w.sent+len(text) > uiShellMax:
		text = text[:uiShellMax-w.sent] + "\n… output truncated"
	}
	w.sent += len(text)
	w.emit(uiEvent{Kind: "shell-out", ID: w.id, Text: text})
	return len(p), nil
}

// Run starts command in dir and returns at once; the rest comes as events. cols is
// how many characters fit in a line of the output (0: the default).
func (s *uiShell) Run(dir, id, command string, cols int) error {
	if cols < 20 || cols > 500 {
		cols = uiShellCols
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	cmd := exec.Command(sh, "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1", "PAGER=cat", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0", fmt.Sprintf("COLUMNS=%d", cols))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Output goes to a terminal, so that programs lay it out for a person (ls in
	// columns); input is empty, so that anything waiting for a keypress ends.
	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: uint16(cols)})
	cmd.Stdout, cmd.Stderr = tty, tty
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return err
	}
	tty.Close() // the child has its own copy
	w := &chunkWriter{emit: s.emit, id: id}
	copied := make(chan struct{})
	go func() {
		io.Copy(w, ptmx) // ends when the last holder of the terminal closes it
		close(copied)
	}()
	s.mu.Lock()
	s.runs[id] = cmd
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		select {
		case <-copied:
		case <-time.After(time.Second): // a background process still holds the terminal
		}
		ptmx.Close()
		s.mu.Lock()
		delete(s.runs, id)
		s.mu.Unlock()
		code := 0
		if err != nil {
			code = -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		e := uiEvent{Kind: "shell-done", ID: id, Turns: code}
		if err != nil && code == -1 {
			e.Text = fmt.Sprint(err)
		}
		s.emit(e)
	}()
	return nil
}

// Stop ends a running command: politely first, then for good.
func (s *uiShell) Stop(id string) {
	s.mu.Lock()
	cmd := s.runs[id]
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	time.AfterFunc(2*time.Second, func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
}

// StopAll ends everything still running, when the window closes.
func (s *uiShell) StopAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.runs))
	for id := range s.runs {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.Stop(id)
	}
}
