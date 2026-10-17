package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// message is one entry of the chat log.
type message struct {
	user bool // typed by the user, otherwise a note from v
	text string
}

var (
	userStyle = bg.Foreground(lipgloss.Color(colFg))
	noteStyle = bg.Foreground(lipgloss.Color(colMuted))
	markStyle = bg.Foreground(lipgloss.Color(colBlue))
)

// note adds a message from v to the chat log.
func (m *model) note(text string) { m.log = append(m.log, message{text: text}) }

// renderLog draws the chat log bottom-aligned in rows lines, wrapping long messages.
func (m *model) renderLog(rows int) []string {
	var lines []string
	for i, msg := range m.log {
		st, mark := noteStyle, bg.Render("  ")
		if msg.user {
			st, mark = userStyle, markStyle.Render("› ")
			if i > 0 {
				lines = append(lines, "")
			}
		}
		wrapped := lipgloss.NewStyle().Width(max(m.w-4, 1)).Render(msg.text)
		for j, l := range strings.Split(wrapped, "\n") {
			if j > 0 {
				mark = bg.Render("  ")
			}
			lines = append(lines, bg.Render(" ")+mark+st.Render(strings.TrimRight(l, " ")))
		}
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	out := make([]string, rows-len(lines), rows)
	out = append(out, lines...)
	for i, l := range out {
		out[i] = l + bg.Render(strings.Repeat(" ", max(m.w-lipgloss.Width(l), 0)))
	}
	return out
}
