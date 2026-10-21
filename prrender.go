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

func rawLine(s string) []seg { return []seg{{s, tokRaw}} }

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
func messageBox(entries []boxEntry, quiet bool, width int) previewLines {
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
		out = append(out, rawLine(bg.Render("  ")+head))
		for _, l := range markdownBody(e.body, width-6) {
			out = append(out, rawLine(bg.Render("  ")+frame.Render("│ ")+l))
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
	return messageBox(entries, t.Resolved, width)
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
