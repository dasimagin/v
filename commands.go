package main

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
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
	Complete func(arg string) (keep string, cands []string)
	Run      func(m *model, arg string) tea.Cmd
}

var commands = []*Command{
	{
		Name:     "open",
		Args:     "[path]",
		Desc:     "Open a file or list a directory",
		Complete: completePath,
		Run:      (*model).cmdOpen,
	},
	{
		Name: "config",
		Desc: "Settings",
		Run:  (*model).cmdConfig,
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
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if m.mode == modeSplash {
		m.mode = modeChat
	}
	m.log = append(m.log, message{user: true, text: input})
	if !strings.HasPrefix(input, "/") {
		m.note("the agent is not connected yet")
		return nil
	}
	name, arg, _ := strings.Cut(input[1:], " ")
	c := findCommand(name)
	if c == nil {
		m.setErr("unknown command /" + name)
		return nil
	}
	return c.Run(m, strings.TrimSpace(arg))
}

// cmdOpen opens a file, or lists a directory; no argument lists the working directory.
func (m *model) cmdOpen(arg string) tea.Cmd {
	path := expandHome(arg)
	if path == "" {
		path = "."
	}
	fi, err := os.Stat(path)
	if err != nil {
		m.setErr(err.Error())
		return nil
	}
	if fi.IsDir() {
		b, err := NewBrowser(path, "")
		if err != nil {
			m.setErr(err.Error())
			return nil
		}
		m.push()
		m.browser, m.mode = b, modeBrowse
		return nil
	}
	m.openFile(path)
	return nil
}

// cmdReset drops the session and shows the splash with a new tagline.
func (m *model) cmdReset(string) tea.Cmd {
	m.log, m.doc, m.browser, m.stack = nil, nil, nil, nil
	m.mode, m.status = modeSplash, ""
	next := rand.IntN(len(taglines) - 1)
	if next >= m.tagline {
		next++
	}
	m.tagline = next
	return nil
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

// completePath completes a file system path; directories end with "/".
func completePath(arg string) (string, []string) {
	keep, prefix := filepath.Split(arg)
	dir := expandHome(keep)
	if dir == "" {
		dir = "."
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return keep, nil
	}
	var cands []string
	for _, de := range des {
		name := de.Name()
		if !strings.HasPrefix(name, prefix) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".")) {
			continue
		}
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && fi.IsDir() {
			name += "/"
		}
		cands = append(cands, name)
	}
	sort.Strings(cands)
	return keep, cands
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
