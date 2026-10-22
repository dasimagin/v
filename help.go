package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// helpHint is what the bottom line shows when it has nothing else to say.
const helpHint = "? help"

type helpRow struct{ keys, what string }

// helpFor is the help of the screen on top: a title and what the keys do there.
func (m *model) helpFor() (string, []helpRow) {
	global := []helpRow{{"?", "this help (with an empty input)"}, {"ctrl+c ×2", "quit"}}
	switch {
	case m.perm != nil || m.reviewAsking():
		return "A question for you", []helpRow{
			{"↑ ↓  enter", "choose and answer"},
			{"y  n  a  x", "answer by letter (a: accept the rest, x: stop)"},
			{"tab", "add a note that goes to the agent with your answer"},
			{"esc", "no"},
		}
	case m.mode == modePick && m.picker != nil:
		return m.pickerHelp(global)
	case m.fileMode():
		if m.doc != nil && m.doc.Ed.On {
			return "Editing a file", append([]helpRow{
				{"ctrl+s", "save"}, {"ctrl+z  ctrl+y", "undo and redo"}, {"ctrl+t", "view mode"}, {"esc", "stop editing"},
			}, global...)
		}
		return "A file", append([]helpRow{
			{"↑ ↓ ← →  pgup pgdn", "scroll"}, {"enter  click", "edit"}, {"tab  ctrl+t", "flat, unified or side-by-side diff"},
			{"ctrl+s", "save"}, {"esc", "back"},
		}, global...)
	case m.mode == modeConfig:
		return "Settings", append([]helpRow{{"type", "filter the settings (fuzzy); esc clears the filter"}, {"↑ ↓  pgup pgdn", "move"}, {"enter  →", "open a dropdown, enter picks"}, {"click", "a row to focus it, its value to open the dropdown, an entry to pick it"}, {"esc  ←", "close a dropdown, then go back"}}, global...)
	case m.mode == modeSessions:
		return "Sessions", append([]helpRow{{"↑ ↓", "move"}, {"enter", "continue the session"}, {"esc", "back"}}, global...)
	case m.runScr != nil:
		return strings.Title(m.runScr.kind) + " run", append([]helpRow{
			{"type + enter", "guide the agent: while it works the note reaches it with its next tool result, after it, a new turn"},
			{"esc", "stop the agent; when it is idle, leave the run and get your chat back"},
			{"ctrl+o", "show or hide its thinking"},
		}, global...)
	}
	return "Chat", append([]helpRow{
		{"/", "commands: /pr  /review  /apply  /open  /log  /branch  /workdir  /config"},
		{"↑ ↓", "history of what you typed"},
		{"ctrl+b  ctrl+p", "branches, the PR"},
		{"ctrl+t", "view mode of diffs (flat, unified, side-by-side)"},
		{"ctrl+o", "show or hide the agent's thinking"},
		{"esc", "interrupt the agent"},
		{"click", "the model and effort below open dropdowns; so do the branch and the PR"},
	}, global...)
}

func (m *model) pickerHelp(global []helpRow) (string, []helpRow) {
	p := m.picker
	if p.compose != nil {
		return "Writing a comment", []helpRow{{"enter", "send it"}, {"esc", "cancel"}, {"click a diff line", "pick the line; click another to make a range"}}
	}
	list := []helpRow{{"type", "filter the list"}, {"↑ ↓", "select"}, {"esc", "back"}}
	switch p.kind {
	case "workdir":
		return "Choose workdir", append([]helpRow{{"type", "filter the folders"}, {"↑ ↓", "select"}, {"→", "open the folder"}, {"←", "go up"}, {"enter", "choose it"}, {"esc", "back"}}, global...)
	case "open":
		return "Open a file", append([]helpRow{{"type", "filter; a name with no match is created"}, {"↑ ↓", "select"}, {"→", "open the folder or the file"}, {"←", "go up"}, {"enter", "open"}, {"shift+↑↓  wheel", "scroll the preview"}, {"esc", "back"}}, global...)
	case "switch":
		return "Switch branch", append(append(list, helpRow{"tab", "cycle the matches"}, helpRow{"enter", "switch"}, helpRow{"shift+↑↓  wheel", "scroll the history"}), global...)
	case "show":
		return "Branch history", append(append(list, helpRow{"shift+↑↓  wheel", "scroll the patch"}), global...)
	case "prdiff":
		return "Pull request", append([]helpRow{
			{"type", "search the list"}, {"↑ ↓", "select a file or a comment"}, {"shift+↑↓  wheel", "scroll the diff"},
			{"ctrl+l", "list: files or comments"}, {"ctrl+r", "reply to the selected thread"}, {"ctrl+n", "new comment (or the review summary from Conversation)"},
			{"ctrl+e", "resolve or reopen the thread"}, {"ctrl+a", "have the agent fix the thread"}, {"ctrl+s", "submit your review: comment, approve, request changes, discard"},
			{"click", "buttons on threads, a diff line (click another for a range), the ✕ on your comments, Submit and the list in the header"},
			{"esc", "back"},
		}, global...)
	}
	return "List", append(list, global...)
}

// overlayHelp draws the help of the screen in the middle of the view.
func (m *model) overlayHelp(lines []string) []string {
	title, rows := m.helpFor()
	keyW := 0
	for _, r := range rows {
		keyW = max(keyW, lipgloss.Width(r.keys))
	}
	inner := min(max(m.w-8, 20), 96)
	panel := bg.Background(lipgloss.Color(cur.Panel))
	text := panel.Foreground(lipgloss.Color(cur.Fg))
	key := panel.Foreground(lipgloss.Color(cur.Accent)).Bold(true)
	muted := panel.Foreground(lipgloss.Color(cur.Muted))

	var body []string
	body = append(body, fgPanel(" "+title, inner, panel.Foreground(lipgloss.Color(cur.Fg)).Bold(true)))
	body = append(body, fgPanel("", inner, text))
	for _, r := range rows {
		k := key.Render(" " + r.keys + strings.Repeat(" ", keyW-lipgloss.Width(r.keys)) + "  ")
		wrapped := strings.Split(ansi.Wrap(r.what, max(inner-keyW-4, 10), ""), "\n")
		for i, w := range wrapped {
			prefix := k
			if i > 0 {
				prefix = panel.Render(strings.Repeat(" ", keyW+3))
			}
			rest := text.Render(w)
			pad := panel.Render(strings.Repeat(" ", max(inner-keyW-3-lipgloss.Width(w), 0)))
			body = append(body, prefix+rest+pad)
		}
	}
	body = append(body, fgPanel("", inner, text), fgPanel(" esc or ? closes this help", inner, muted))
	top := max((len(lines)-len(body))/2, 1)
	left := max((m.w-inner)/2, 0)
	for i, l := range body {
		y := top + i
		if y >= len(lines)-1 {
			break
		}
		lines[y] = ansi.Cut(lines[y], 0, left) + l + ansi.Cut(lines[y], left+inner, m.w)
	}
	return lines
}

// fgPanel is a padded row of text on the panel background.
func fgPanel(s string, width int, st lipgloss.Style) string {
	return st.Render(s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0)))
}
