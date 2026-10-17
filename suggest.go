package main

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// menu is the suggestion list under the omnibox. Command names show up as soon
// as the line starts with "/"; arguments show up on Tab. Typing re-filters,
// Tab/Shift-Tab/↑↓ move the selection, Enter accepts, Esc hides.
type menu struct {
	head  string // omnibox text before the word being completed
	items []item
	sel   int
	args  bool // completing an argument rather than a command name
}

type item struct {
	value string // what replaces the word
	label string
	desc  string
	cmd   *Command // set for command items
}

const menuRows = 6

// suggest rebuilds the menu for the current omnibox value. force opens an
// argument menu; otherwise only a command menu or an already open one updates.
func (m *model) suggest(force bool) {
	v := m.omni.Value()
	if !force && v == m.dismissed {
		m.menu = nil
		return
	}
	prev := m.menu
	m.menu = nil
	if !strings.HasPrefix(v, "/") {
		return
	}
	name, arg, hasArg := strings.Cut(v[1:], " ")
	if !hasArg {
		mu := &menu{head: "/"}
		for _, c := range commands {
			if !c.matchesPrefix(name) {
				continue
			}
			label := "/" + c.Name
			if c.Args != "" {
				label += " " + c.Args
			}
			desc := c.Desc
			if len(c.Aliases) > 0 {
				desc += " (/" + strings.Join(c.Aliases, ", /") + ")"
			}
			mu.items = append(mu.items, item{value: c.Name, label: label, desc: desc, cmd: c})
		}
		m.setMenu(mu, prev)
		return
	}
	c := findCommand(name)
	if c == nil || c.Complete == nil || (!force && (prev == nil || !prev.args)) {
		return
	}
	keep, cands := c.Complete(arg)
	mu := &menu{head: "/" + name + " " + keep, args: true}
	for _, s := range cands {
		mu.items = append(mu.items, item{value: s, label: s})
	}
	m.setMenu(mu, prev)
}

// setMenu installs mu, keeping the previous selection if it is still listed.
func (m *model) setMenu(mu, prev *menu) {
	if len(mu.items) == 0 {
		return
	}
	if prev != nil && prev.args == mu.args && len(prev.items) > 0 {
		want := prev.items[prev.sel].value
		for i, it := range mu.items {
			if it.value == want {
				mu.sel = i
			}
		}
	}
	m.menu = mu
}

// tab handles Tab (dir=1) and Shift-Tab (dir=-1).
func (m *model) tab(dir int) {
	if m.menu != nil {
		m.moveSel(dir)
		return
	}
	m.suggest(true)
	mu := m.menu
	if mu == nil {
		return
	}
	if mu.args && len(mu.items) == 1 {
		m.accept()
		return
	}
	// Extend the typed word to what all candidates share, like a shell.
	word := strings.TrimPrefix(m.omni.Value(), mu.head)
	if p := commonPrefix(mu.items); len(p) > len(word) {
		m.setOmni(mu.head + p)
		m.suggest(false)
	}
}

func (m *model) moveSel(dir int) {
	n := len(m.menu.items)
	m.menu.sel = ((m.menu.sel+dir)%n + n) % n
}

// accept applies the selected item. A command that takes no argument, or one
// already typed in full, runs right away.
func (m *model) accept() tea.Cmd {
	mu := m.menu
	it := mu.items[mu.sel]
	m.menu = nil
	if c := it.cmd; c != nil {
		if c.Args == "" || findCommand(strings.TrimPrefix(m.omni.Value(), "/")) == c {
			m.omni.SetValue("")
			return m.run("/" + c.Name)
		}
		m.setOmni("/" + c.Name + " ")
		return nil
	}
	m.setOmni(mu.head + it.value)
	if strings.HasSuffix(it.value, "/") {
		m.suggest(true) // step into the directory
	}
	return nil
}

func (m *model) setOmni(v string) {
	m.omni.SetValue(v)
	m.omni.CursorEnd()
}

func commonPrefix(items []item) string {
	p := items[0].value
	for _, it := range items[1:] {
		for !strings.HasPrefix(it.value, p) {
			_, n := utf8.DecodeLastRuneInString(p)
			p = p[:len(p)-n]
		}
	}
	return p
}

var (
	menuSel  = bg.Foreground(lipgloss.Color(colBlue)).Bold(true)
	menuItem = bg.Foreground(lipgloss.Color(colFg))
	menuDir  = bg.Foreground(lipgloss.Color(colBlue))
)

// renderMenu draws up to menuRows items, scrolled to keep the selection visible.
func (m *model) renderMenu(width int) []string {
	mu := m.menu
	n := min(len(mu.items), menuRows)
	top := min(max(mu.sel-n+1, 0), len(mu.items)-n)
	labelW := 0
	for _, it := range mu.items {
		labelW = max(labelW, lipgloss.Width(it.label))
	}
	out := make([]string, 0, n)
	for i := top; i < top+n; i++ {
		it := mu.items[i]
		mark, st := "  ", menuItem
		if mu.args && strings.HasSuffix(it.value, "/") {
			st = menuDir
		}
		if i == mu.sel {
			mark, st = "› ", menuSel
		}
		line := st.Render(" "+mark+it.label+strings.Repeat(" ", labelW-lipgloss.Width(it.label))) +
			hintStyle.Render("   "+it.desc)
		out = append(out, line+bg.Render(strings.Repeat(" ", max(width-lipgloss.Width(line), 0))))
	}
	return out
}
