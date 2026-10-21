package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// message is one entry of the chat log.
type message struct {
	kind   msgKind
	text   string
	answer string // for msgAsk once answered: "y", "n" or "e" (saved the user's version)

	// A review comment (msgFinding): where, the diff around it, and the commented line in it.
	file   string
	hunk   []string
	target int

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
	msgAsk                     // a tool call waiting for, or given, the user's answer
	msgThinking                // the model's thinking, collapsed unless expanded
	msgError                   // an error from the agent or a tool
	msgFinding                 // a review comment with its diff, waiting for y or n
)

// note adds a message from v to the chat log.
func (m *model) note(text string) { m.log = append(m.log, message{kind: msgNote, text: text}) }

// noteErr adds an error to the chat log.
func (m *model) noteErr(text string) { m.log = append(m.log, message{kind: msgError, text: text}) }

// toolLine draws "Name argument" with the name in name and the rest in arg.
func toolLine(text string, name, arg lipgloss.Style) string {
	n, a, _ := strings.Cut(text, " ")
	out := name.Bold(true).Render(n)
	if a != "" {
		out += arg.Render(" " + a)
	}
	return out
}

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
			// The agent's own actions stay in the background.
			lines = append(lines, chatLine{dim.Render("⏺ "), toolLine(msg.text, hintStyle, dim), bg})
		case msgAsk:
			// "?" while waiting, then the answer: ✓ yes, ✗ no, ✎ the user's version.
			switch msg.answer {
			case "":
				ask := bg.Foreground(c(cur.Type)).Bold(true)
				lines = append(lines, chatLine{ask.Render("? "), ask.Render("Allow " + msg.text + "?"), bg})
			case "y":
				lines = append(lines, chatLine{bg.Foreground(c(cur.Added)).Render("✓ "), toolLine(msg.text, hintStyle, dim), bg})
			case "n":
				lines = append(lines, chatLine{errStyle.Render("✗ "), toolLine(msg.text, hintStyle, dim), bg})
			case "e":
				lines = append(lines, chatLine{accentStyle.Render("✎ "), toolLine(msg.text, hintStyle, dim) + dim.Render(" · your version"), bg})
			}
		case msgFinding:
			gap()
			mark := bg.Foreground(c(cur.Type)).Bold(true).Render("? ")
			switch msg.answer {
			case "y":
				mark = bg.Foreground(c(cur.Added)).Render("✓ ")
			case "n":
				mark = errStyle.Render("✗ ")
			}
			lines = append(lines, chatLine{mark, bg.Foreground(c(cur.Fg)).Bold(true).Render(truncate(msg.file, width)), bg})
			comment := func() {
				bar := bg.Foreground(c(cur.Accent)).Render("┃ ")
				mark, text := splitLevel(msg.text)
				if mark != "" {
					text = mark + " " + text // wrapped with its text; drawn bold below
				}
				for j, l := range strings.Split(lipgloss.NewStyle().Width(max(width-4, 1)).Render(text), "\n") {
					l = strings.TrimRight(l, " ")
					body := bg.Foreground(c(cur.Fg)).Render(l)
					if rest, ok := strings.CutPrefix(l, mark); ok && j == 0 && mark != "" {
						st := bg.Foreground(c(cur.Accent)).Bold(true)
						if mark == "!!" {
							st = bg.Foreground(c(cur.Deleted)).Bold(true)
						}
						body = st.Render(mark) + bg.Foreground(c(cur.Fg)).Render(rest)
					}
					lines = append(lines, chatLine{dim.Render("│ "), bar + body, bg})
				}
			}
			if len(msg.hunk) == 0 {
				comment() // not on a diff line
			}
			for j, l := range msg.hunk {
				st := dim
				switch {
				case strings.HasPrefix(l, "+"):
					st = bg.Foreground(c(cur.Added))
				case strings.HasPrefix(l, "-"):
					st = bg.Foreground(c(cur.Deleted))
				}
				if j == msg.target {
					st = st.Bold(true)
				}
				lines = append(lines, chatLine{dim.Render("│ "), st.Render(truncate(strings.ReplaceAll(l, "\t", "    "), width-2)), bg})
				if j == msg.target {
					comment() // right under the line it is about
				}
			}
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
