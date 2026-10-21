package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// A tip is a small card on the start screen: a sketch of a screen and a line about it.
type tip struct {
	title   string
	lines   []string // a leading marker colors the line: ">" accent, "+" green, "-" red, "~" muted, "*" bold
	caption string
}

var tips = []tip{
	{
		title: "/pr · read a PR, answer its threads",
		lines: []string{
			"> Submit [2] ▾   Files [3] ▾",
			"~ ╭─ @alice · 2h ago  ○ unresolved   Reply  Apply",
			"  │ Extract this block into a helper.",
			"~ ├─ ↳ @you · 1h ago",
			"  │ Done, pushed in 6864f4e.",
			"~ ╰─",
		},
		caption: "Threads are boxes with buttons. What you write waits in your pending review until you Submit it with a verdict.",
	},
	{
		title: "/review make · a fresh agent reviews the PR",
		lines: []string{
			"~ ? git.go:42",
			"~    41   func load() {",
			"+    42 ▎ x := read()",
			"  │ ╭─ Claude",
			"  │ │ !! This drops the error",
			"  │ ╰─",
			"> › Yes, post it",
			"~   No, skip it",
		},
		caption: "You accept its comments one by one, or let it keep them as a pending review. Guide it any time you like.",
	},
	{
		title: "/apply · fix the open threads",
		lines: []string{
			"> Apply · PR #7",
			"~ ⏺ Read git.go",
			"~ ⏺ Edit git.go",
			"~ ⏺ Bash go test ./...",
			"+ ✓ reply · git.go:42 (@alice)",
			"+ ✓ resolve · git.go:42 (@alice)",
		},
		caption: "An agent of its own edits, verifies, commits, pushes, replies and resolves. Esc brings your chat back.",
	},
	{
		title: "/open · find a file, see it before you open it",
		lines: []string{
			"> /open internal/",
			"* › picker.go           ┆ package main",
			"    preview.go          ┆ ",
			"    workdir.go          ┆ import (",
			"~   → into a folder     ┆     \"fmt\"",
		},
		caption: "Fuzzy search with a read-only preview. → goes into a folder, ← up, Enter opens.",
	},
	{
		title: "/show · the history of the branch",
		lines: []string{
			"> /show",
			"* › 6864f4e  Dedupe remote branches  ┆ + func dedupe(",
			"    a1b2c3d  Add /apply              ┆ - old := list",
			"    9f8e7d6  Fix the reply endpoint  ┆ @@ -12,4 +12,6 @@",
		},
		caption: "Type to search commits; the patch of the selected one is on the right. /switch previews a branch's log the same way.",
	},
	{
		title: "model and effort · below, on the left",
		lines: []string{
			"~ Opus 5.5 · high   ? help                  ~/proj   feat   PR #7",
			"> ▸ Fable 5.1",
			"* › Opus 5.5",
			"    Sonnet 5.5",
		},
		caption: "Click the model or the effort for a dropdown. Press ? on any screen for its keys.",
	},
}

// styleTipLine colors a sketch line by its first character, which is followed
// by a space; both are dropped. A space there leaves the line plain.
func styleTipLine(l string) string {
	if len(l) < 2 {
		return ""
	}
	st, text := fgStyle, l[2:]
	switch l[0] {
	case '>':
		st = accentStyle.Bold(true)
	case '+':
		st = bg.Foreground(lipgloss.Color(cur.Added))
	case '-':
		st = bg.Foreground(lipgloss.Color(cur.Deleted))
	case '~':
		st = hintStyle
	case '*':
		st = fgStyle.Bold(true)
	}
	return st.Render(text)
}

// renderTip draws one card at the given width: a title, the sketch in a frame, the caption.
func renderTip(t tip, width int) []string {
	frame := hintStyle
	inner := max(width-4, 10)
	out := []string{accentStyle.Bold(true).Render(truncate(t.title, width)), ""}
	out = append(out, frame.Render("╭"+strings.Repeat("─", inner+2)+"╮"))
	for _, l := range t.lines {
		s := ansi.Truncate(styleTipLine(l), inner, "…")
		out = append(out, frame.Render("│ ")+s+bg.Render(strings.Repeat(" ", max(inner-lipgloss.Width(s), 0)))+frame.Render(" │"))
	}
	out = append(out, frame.Render("╰"+strings.Repeat("─", inner+2)+"╯"), "")
	for _, l := range strings.Split(ansi.Wrap(t.caption, width, ""), "\n") {
		out = append(out, hintStyle.Italic(true).Render(l))
	}
	return out
}

// splash draws the start screen in width×rows: the logo with its tagline on the
// left and, when the window is wide enough, a rotating tip on the right.
func splash(tagline string, n, width, rows int) []string {
	logoLines := renderLogo()
	// A fixed width keeps the card where it is from one joke to the next.
	const leftW = 36
	taglineW := leftW
	// The logo and the joke under it are centered on one axis, whatever the joke's length.
	var left []string
	for _, l := range logoLines {
		left = append(left, centerIn(l, leftW))
	}
	left = append(left, "")
	for _, l := range strings.Split(ansi.Wrap(tagline, taglineW, ""), "\n") {
		left = append(left, centerIn(hintStyle.Italic(true).Render(strings.TrimSpace(l)), leftW))
	}

	const gap = 8
	cardW := min(width-leftW-gap-4, 60)
	if cardW < 36 { // too narrow for two columns: the logo alone, centered
		return centered(left, width, rows)
	}
	right := renderTip(tips[n%len(tips)], cardW)

	total := leftW + gap + cardW
	margin := max((width-total)/2, 0)
	height := max(len(left), len(right))
	top := max((rows-height)/2, 0)
	out := make([]string, rows)
	for i := range out {
		line := bg.Render(strings.Repeat(" ", margin))
		var l, r string
		if j := i - top; j >= 0 && j < height {
			// the logo is centered against the card, so the two read as one composition
			if k := j - (height-len(left))/2; k >= 0 && k < len(left) {
				l = left[k]
			}
			if j < len(right) {
				r = right[j]
			}
		}
		pad := bg.Render(strings.Repeat(" ", max(leftW-lipgloss.Width(l), 0)))
		line += l + pad + bg.Render(strings.Repeat(" ", gap)) + r
		out[i] = line + bg.Render(strings.Repeat(" ", max(width-lipgloss.Width(line), 0)))
	}
	return out
}

// centered puts a block of lines in the middle of width×rows.
func centered(block []string, width, rows int) []string {
	top := max((rows-len(block))/2, 0)
	out := make([]string, rows)
	for i := range out {
		line := ""
		if i >= top && i < top+len(block) {
			line = block[i-top]
		}
		out[i] = center(line, width)
	}
	return out
}

// centerIn pads a rendered line to width, centered.
func centerIn(s string, width int) string {
	w := lipgloss.Width(s)
	left := max((width-w)/2, 0)
	return bg.Render(strings.Repeat(" ", left)) + s + bg.Render(strings.Repeat(" ", max(width-left-w, 0)))
}
