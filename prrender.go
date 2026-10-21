package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tokRaw marks a preview line that is already styled text, drawn as it is.
const tokRaw chroma.TokenType = -1

// commentButton styles the buttons that start a comment or reply.
var commentButton lipgloss.Style

func rawLine(s string) []seg { return []seg{{s, tokRaw}} }

// colorMark recolors the importance mark ("!!" red, "!" accent) at the start
// of a rendered comment line, and returns to the panel background after it.
func colorMark(line, mark string) string {
	color := cur.Accent
	if mark == "!!" {
		color = cur.Deleted
	}
	st := bg.Background(lipgloss.Color(cur.Panel)).Foreground(lipgloss.Color(color)).Bold(true)
	return strings.Replace(line, mark, st.Render(mark)+bgSeq(cur.Panel), 1)
}

// markdownBody renders a comment as terminal lines, without the blank rows
// around it.
func markdownBody(text string, width int) []string {
	lines := markdown(strings.TrimSpace(text), max(width, 10))
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type badge struct {
	text  string
	style lipgloss.Style
}

func (b badge) render() string { return bg.Render(" ") + b.style.Render("["+b.text+"]") }

// boxEntry is one message of a box: who wrote it, when, and what.
type boxEntry struct {
	author string
	at     time.Time
	body   string
	badges []badge
}

// messageBox draws messages in a rounded frame, replies indented under the
// first message. The frame takes the muted color when quiet is set.
func messageBox(entries []boxEntry, quiet bool, width int, button string) previewLines {
	frame := accentStyle
	if quiet {
		frame = hintStyle
	}
	author := accentStyle.Bold(true)
	var out previewLines
	for i, e := range entries {
		corner := "╭─ "
		if i > 0 {
			corner = "├─ ↳ "
		}
		head := frame.Render(corner) + author.Render("@"+e.author)
		if !e.at.IsZero() {
			head += hintStyle.Render(" · " + ago(e.at))
		}
		for _, b := range e.badges {
			head += b.render()
		}
		if i == 0 && button != "" { // right-aligned, so it is easy to find and click
			head += bg.Render(strings.Repeat(" ", max(width-1-lipgloss.Width(head)-lipgloss.Width(button)-2, 1))) + button
		}
		out = append(out, rawLine(bg.Render("  ")+head))
		panel := bg.Background(lipgloss.Color(cur.Panel))
		body := markdownOn(strings.TrimSpace(e.body), width-7)
		if mark, _ := splitLevel(e.body); mark != "" && len(body) > 0 {
			body[0] = colorMark(body[0], mark)
		}
		for _, l := range body {
			out = append(out, rawLine(bg.Render("  ")+frame.Render("│")+panel.Render(" ")+l+panel.Render(" ")))
		}
	}
	return append(out, rawLine(bg.Render("  ")+frame.Render("╰─")))
}

// threadBox draws a review thread with its state.
func threadBox(t prThread, width int) previewLines {
	var entries []boxEntry
	for i, c := range t.Comments {
		e := boxEntry{author: c.Author, at: c.At, body: c.Body}
		if i == 0 {
			switch {
			case t.Resolved:
				e.badges = append(e.badges, badge{"resolved", bg.Foreground(lipgloss.Color(cur.Added))})
			default:
				e.badges = append(e.badges, badge{"unresolved", bg.Foreground(lipgloss.Color(cur.Type))})
			}
			if t.Outdated {
				e.badges = append(e.badges, badge{"outdated", hintStyle})
			}
		}
		if c.Pending {
			e.badges = append(e.badges, badge{"pending", bg.Foreground(lipgloss.Color(cur.Type)).Bold(true)})
		}
		entries = append(entries, e)
	}
	button := ""
	if t.ID != "" {
		reply, resolve := threadButtons(t)
		button = reply + bg.Render(" ") + resolve
	}
	return messageBox(entries, t.Resolved, width, button)
}

// threadButtons are the Reply and Resolve (or Unresolve) buttons of a thread.
func threadButtons(t prThread) (reply, resolve string) {
	st := buttonStyle
	if t.Resolved {
		st = bg.Background(lipgloss.Color(cur.Sel)).Foreground(lipgloss.Color(cur.Fg)).Bold(true)
	}
	return bg.Background(lipgloss.Color(cur.Sel)).Foreground(lipgloss.Color(cur.Fg)).Bold(true).Render(" Reply "), st.Render(threadButtonLabel(t))
}

// threadHits are where those buttons sit in a preview of the given width,
// relative to its left edge: they are right-aligned in the thread's header row.
func threadHits(t prThread, width int) (reply, resolve [2]int) {
	r, s := threadButtons(t)
	rw, sw := lipgloss.Width(r), lipgloss.Width(s)
	right := width - 1
	resolve = [2]int{right - sw, right}
	reply = [2]int{right - sw - 1 - rw, right - sw - 1}
	return
}

// threadButtonLabel is the text of the resolve toggle of a thread.
func threadButtonLabel(t prThread) string {
	if t.Resolved {
		return " Unresolve "
	}
	return " Resolve "
}

func (t prThread) summary() string {
	if len(t.Comments) == 0 {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(t.Comments[0].Body), "\n")
	mark, rest := splitLevel(first)
	if mark != "" {
		rest = mark + " " + rest
	}
	s := fmt.Sprintf("@%s %s", t.Comments[0].Author, rest)
	if n := len(t.Comments) - 1; n > 0 {
		s += fmt.Sprintf("  +%d", n)
	}
	if t.Resolved {
		s += "  · resolved"
	}
	return s
}
