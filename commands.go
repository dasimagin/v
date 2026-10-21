package main

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Command is one omnibox command, typed as /name or /alias.
type Command struct {
	Name    string
	Aliases []string
	Args    string // argument hint for the menu, e.g. "[path]"; "" if it takes none
	Desc    string
	// Complete lists candidates for the argument being typed: keep is the part of
	// arg that stays, cands are the options for the rest. nil if no completion.
	Complete func(dir, arg string) (keep string, cands []string)
	Run      func(m *model, arg string) tea.Cmd
}

var commands = []*Command{
	{
		Name: "workdir", Args: "[directory]", Desc: "Find and choose the working directory",
		Run: (*model).cmdWorkdir,
	},
	{
		Name: "switch", Aliases: []string{"branch"}, Args: "[name]", Desc: "Find and switch Git branches", Run: (*model).cmdSwitch,
	},
	{
		Name: "show", Desc: "Browse this branch's history", Run: (*model).cmdShow,
	},
	{
		Name: "apply", Desc: "Have the agent fix the open review threads of this PR", Run: (*model).cmdApply,
	},
	{
		Name: "review", Args: "[make | submit | discard]", Desc: "Pending review status; make runs a Claude review, submit or discard its pending review", Run: (*model).cmdReview,
	},
	{
		Name: "pr", Args: "[URL | number | auto]", Desc: "Read this branch's PR, or choose or link one", Run: (*model).cmdPR,
	},
	{
		Name: "open",
		Args: "[path]",
		Desc: "Find and open a file",
		Run:  (*model).cmdOpen,
	},
	{
		Name: "config",
		Desc: "Settings",
		Run:  (*model).cmdConfig,
	},
	{
		Name: "resume",
		Desc: "Continue a past session in this directory",
		Run:  (*model).cmdResume,
	},
	{
		Name: "reset",
		Desc: "Start a new session",
		Run:  (*model).cmdReset,
	},
	{
		Name:    "quit",
		Aliases: []string{"q"},
		Desc:    "Quit v",
		Run:     func(*model, string) tea.Cmd { return tea.Quit },
	},
}

func findCommand(name string) *Command {
	for _, c := range commands {
		if c.Name == name {
			return c
		}
		for _, a := range c.Aliases {
			if a == name {
				return c
			}
		}
	}
	return nil
}

// matchesPrefix reports whether the command's name or an alias starts with p.
func (c *Command) matchesPrefix(p string) bool {
	if strings.HasPrefix(c.Name, p) {
		return true
	}
	for _, a := range c.Aliases {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

// run executes an omnibox line.
func (m *model) run(input string) tea.Cmd {
	if m.repoChanging {
		m.setErr("wait for the Git operation to finish")
		return nil
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if m.mode == modeSplash {
		m.mode = modeChat
	}
	m.log = append(m.log, message{kind: msgUser, text: input})
	if !strings.HasPrefix(input, "/") && m.rv != nil && !m.rv.apply && m.rv.running && !m.rv.dropped {
		m.rv.addNote(input) // steers the review, not the chat agent
		m.note("note for the reviewer · it sees it with its next tool result")
		return nil
	}
	if !strings.HasPrefix(input, "/") {
		// The reply streams into the chat, so show it.
		if m.mode != modeChat {
			m.push()
			m.mode = modeChat
		}
		return m.prompt(input)
	}
	name, arg, _ := strings.Cut(input[1:], " ")
	c := findCommand(name)
	if c == nil {
		m.setErr("unknown command /" + name)
		return nil
	}
	return c.Run(m, strings.TrimSpace(arg))
}

func (m *model) cmdOpen(arg string) tea.Cmd { return m.openPicker("open", arg) }

// openPath opens a file; a missing one is created empty, like `touch`.
func (m *model) openPath(path string) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := createFile(path); err != nil {
			m.setErr(err.Error())
			return
		}
		m.note("created " + shortPath(path, m.workdir))
	} else if err != nil {
		m.setErr(err.Error())
		return
	}
	m.openFile(path)
}

// cmdReset drops the session and shows the splash with a new tagline.
func (m *model) cmdReset(string) tea.Cmd {
	m.stopAgent()
	m.sessionID, m.resumeNext = "", false
	m.log, m.doc, m.stack = nil, nil, nil
	m.picker = nil
	m.mode, m.status = modeSplash, ""
	next := rand.IntN(len(taglines) - 1)
	if next >= m.tagline {
		next++
	}
	m.tagline = next
	return nil
}

// createFile makes an empty file. Its directory must exist: a typo in a
// directory name should not leave a trail of new directories.
func createFile(path string) error {
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("no such directory: %s", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no such directory: %s", filepath.Dir(path))
	}
	if err != nil {
		return err
	}
	return f.Close()
}

func (m *model) openFile(path string) {
	d, err := LoadDoc(path)
	if err != nil {
		m.setErr(err.Error())
		return
	}
	m.push()
	m.doc, m.top, m.left, m.mode = d, 0, 0, modeView
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

func tildify(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
