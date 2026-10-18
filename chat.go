package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// message is one entry of the chat log.
type message struct {
	kind msgKind
	text string

	// Rendered markdown of an agent reply, reused while text, width and
	// theme stay the same.
	md      []string
	mdKey   string
	mdTheme *Theme
}

type msgKind int

const (
	msgUser     msgKind = iota // typed by the user
	msgNote                    // a status line from v
	msgAgent                   // the agent's reply
	msgTool                    // a tool call by the agent: "Name argument"
	msgAsk                     // a permission question
	msgThinking                // the model's thinking, collapsed unless expanded
	msgError                   // an error from the agent or a tool
)

// note adds a message from v to the chat log.
func (m *model) note(text string) { m.log = append(m.log, message{kind: msgNote, text: text}) }

// noteErr adds an error to the chat log.
func (m *model) noteErr(text string) { m.log = append(m.log, message{kind: msgError, text: text}) }

// chatLine is one rendered row: a two-cell mark and the text, plus the
// background the row is padded with.
type chatLine struct {
	mark, text string
	pad        lipgloss.Style
}

// renderLog draws the chat log bottom-aligned in rows lines, wrapping long messages.
func (m *model) renderLog(rows int) []string {
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	dim := bg.Foreground(c(cur.Gutter)) // system notes: dimmer than hints
	userBand := lipgloss.NewStyle().Background(c(cur.Sel))
	width := max(m.w-4, 1)

	var lines []chatLine
	add := func(mark string, text string, st lipgloss.Style, pad lipgloss.Style) {
		blankMark := pad.Render("  ")
		for j, l := range strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n") {
			if j > 0 {
				mark = blankMark
			}
			lines = append(lines, chatLine{mark, st.Render(strings.TrimRight(l, " ")), pad})
		}
	}
	gap := func() { lines = append(lines, chatLine{pad: bg}) }

	for i := range m.log {
		msg := &m.log[i]
		switch msg.kind {
		case msgUser:
			if i > 0 {
				gap()
			}
			add(userBand.Foreground(c(cur.Accent)).Bold(true).Render("› "), msg.text, userBand.Foreground(c(cur.Fg)).Bold(true), userBand)
		case msgAgent:
			gap()
			mark := bg.Foreground(c(cur.Accent2)).Render("● ")
			key := fmt.Sprintf("%d:%s", width, msg.text)
			if msg.mdKey != key || msg.mdTheme != cur {
				msg.md, msg.mdKey, msg.mdTheme = markdown(msg.text, width), key, cur
			}
			for _, l := range msg.md {
				lines = append(lines, chatLine{mark, l, bg})
				mark = bg.Render("  ")
			}
		case msgTool:
			name, arg, _ := strings.Cut(msg.text, " ")
			text := bg.Foreground(c(cur.Func)).Bold(true).Render(name)
			if arg != "" {
				text += hintStyle.Render(" " + arg)
			}
			lines = append(lines, chatLine{bg.Foreground(c(cur.Added)).Render("⏺ "), text, bg})
		case msgAsk:
			add(bg.Foreground(c(cur.Type)).Bold(true).Render("? "), msg.text, bg.Foreground(c(cur.Type)).Bold(true), bg)
		case msgError:
			add(errStyle.Render("✗ "), msg.text, errStyle, bg)
		case msgThinking:
			head := "Thinking"
			if !m.showThinking {
				head += " · ctrl+o to expand"
			}
			add(dim.Render("✻ "), head, dim.Italic(true), bg)
			if m.showThinking && msg.text != "" {
				add(dim.Render("  "), msg.text, hintStyle.Italic(true), bg)
			}
		default:
			add(dim.Render("  "), msg.text, dim, bg)
		}
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	out := make([]string, 0, rows)
	for len(out)+len(lines) < rows {
		out = append(out, blank(m.w))
	}
	for _, l := range lines {
		s := l.pad.Render(" ") + l.mark + l.text
		if l.mark == "" {
			s = ""
		}
		out = append(out, s+l.pad.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(s), 0))))
	}
	return out
}
