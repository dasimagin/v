package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// workPath resolves a path against the workspace without changing process cwd.
func workPath(dir, path string) string {
	path = expandHome(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func workingDirectory(base, path string) (string, error) {
	dir, err := filepath.Abs(workPath(base, path))
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("not a directory: %s", dir)
	}
	return dir, nil
}

// startupPaths accepts v [directory], v [file], or v -C/--workdir dir [file].
// PWD is captured once; all later paths are relative to workdir.
func startupPaths(args []string, out io.Writer) (dir, file string, err error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	fs := flag.NewFlagSet("v", flag.ContinueOnError)
	fs.SetOutput(out)
	var path string
	fs.StringVar(&path, "workdir", "", "working directory (default: current directory)")
	fs.StringVar(&path, "C", "", "working directory (short form)")
	fs.Usage = func() {
		fmt.Fprintln(out, "usage: v [--workdir directory | -C directory] [directory or file]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if fs.NArg() > 1 {
		return "", "", errors.New("expected one directory or file")
	}
	if fs.NArg() == 1 {
		file = fs.Arg(0)
		if path == "" {
			if fi, err := os.Stat(workPath(pwd, file)); err == nil && fi.IsDir() {
				path, file = file, ""
			}
		}
	}
	dir, err = workingDirectory(pwd, path)
	return dir, file, err
}

func (m *model) cmdWorkdir(arg string) tea.Cmd {
	if arg == "" {
		return m.openPicker("workdir", "")
	}
	dir, err := workingDirectory(m.workdir, arg)
	if err != nil {
		m.setErr(err.Error())
		return nil
	}
	if dir == m.workdir {
		m.flash("workdir: " + dir)
		return nil
	}
	if !m.canSwitchWorkspace() {
		return nil
	}
	m.clearWorkspaceViews()
	m.agentStale = m.agent != nil // reconnect in dir with the same session on the next prompt
	m.resumeNext = m.sessionID != ""
	m.workdir, m.repo = dir, nil
	m.workspaceNotice = "The working directory is now " + dir + ". Continue the same conversation in this directory."
	m.repoSeq++ // ignore results from the previous directory
	m.repoLoading, m.prLoading = false, false
	m.flash("workdir: " + tildify(dir))
	return m.refreshRepository(true)
}

func (m *model) canSwitchWorkspace() bool {
	if m.repoChanging {
		m.setErr("wait for the branch switch to finish")
		return false
	}
	if m.busy || m.perm != nil {
		m.setErr("interrupt the agent before switching workdir or branch")
		return false
	}
	dirty := func(d *Doc) bool { return d != nil && d.Ed.Dirty && d.Review == "" }
	if dirty(m.doc) {
		m.setErr("save or close the modified file before switching")
		return false
	}
	for _, s := range m.stack {
		if dirty(s.doc) {
			m.setErr("save or close the modified file before switching")
			return false
		}
	}
	return true
}

func (m *model) clearWorkspaceViews() {
	m.doc, m.stack = nil, nil
	m.picker = nil
	m.sessions, m.cfg, m.viewMenu, m.repoMenu, m.menu = nil, nil, nil, nil, nil
	m.mode, m.top, m.left = modeChat, 0, 0
	m.noting, m.interrupted, m.discardArmed = false, false, false
	m.omni.SetValue("")
	m.omni.Focus()
}
