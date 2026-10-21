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

// boxHits are the clickable spots in the header row of a box, as columns
// [x0, x1) from the left edge of the preview.
type boxHits struct{ state, reply [2]int }

// messageBox draws messages in a rounded frame, replies indented under the
// first message. The frame takes the muted color when quiet is set. The first
// header can carry a state button after its badges and a button at the right
// edge; their positions are returned.
func messageBox(entries []boxEntry, quiet bool, width int, state, button string) (previewLines, boxHits) {
	frame := accentStyle
	if quiet {
		frame = hintStyle
	}
	author := accentStyle.Bold(true)
	var out previewLines
	var hits boxHits
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
		if i == 0 && state != "" {
			head += bg.Render(" ")
			x0 := 2 + lipgloss.Width(head)
			head += state
			hits.state = [2]int{x0, 2 + lipgloss.Width(head)}
		}
		if i == 0 && button != "" { // right-aligned, so it is easy to find and click
			head += bg.Render(strings.Repeat(" ", max(width-1-lipgloss.Width(head)-lipgloss.Width(button)-2, 1))) + button
			right := 2 + lipgloss.Width(head)
			hits.reply = [2]int{right - lipgloss.Width(button), right}
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
	return append(out, rawLine(bg.Render("  ")+frame.Render("╰─"))), hits
}

// threadBox draws a review thread. Its state is a button: clicking it
// resolves or reopens the thread.
func threadBox(t prThread, width int) (previewLines, boxHits) {
	var entries []boxEntry
	for i, c := range t.Comments {
		e := boxEntry{author: c.Author, at: c.At, body: c.Body}
		if i == 0 && t.Start > 0 && t.Start < t.Line {
			e.badges = append(e.badges, badge{fmt.Sprintf("lines %d–%d", t.Start, t.Line), hintStyle})
		}
		if i == 0 && t.Outdated {
			e.badges = append(e.badges, badge{"outdated", hintStyle})
		}
		if c.Pending {
			e.badges = append(e.badges, badge{"pending", bg.Foreground(lipgloss.Color(cur.Type)).Bold(true)})
		}
		entries = append(entries, e)
	}
	state, reply := "", ""
	if t.ID != "" {
		state = stateButton(t)
		reply = replyButton()
	}
	return messageBox(entries, t.Resolved, width, state, reply)
}

// stateButton shows whether a thread is resolved; clicking it flips that.
func stateButton(t prThread) string {
	if t.Resolved {
		return bg.Background(lipgloss.Color(cur.Sel)).Foreground(lipgloss.Color(cur.Added)).Bold(true).Render(" ✓ resolved ")
	}
	return buttonStyle.Render(" ○ unresolved ")
}

func replyButton() string {
	return bg.Background(lipgloss.Color(cur.Sel)).Foreground(lipgloss.Color(cur.Fg)).Bold(true).Render(" Reply ")
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
