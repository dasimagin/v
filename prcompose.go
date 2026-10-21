package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// composeState is a comment being written on the PR screen: the omnibox holds
// its text until Enter sends it or Esc drops it.
type composeState struct {
	kind   string // "reply", "line" or "general"
	thread prThread
	file   prFile
	line   int // last line picked by clicking; 0 when the text starts with the number
	start  int // first line of a picked range, equal to line for a single line
	anchor int // the line first clicked: further clicks extend the range from it
	hint   string
}

// lines is the picked range as text: "42" or "100–120".
func (c *composeState) lines() string {
	if c.start > 0 && c.start < c.line {
		return fmt.Sprintf("%d–%d", c.start, c.line)
	}
	return fmt.Sprint(c.line)
}

func (c *composeState) setHint() {
	c.hint = fmt.Sprintf("comment on %s:%s · click another line to extend the range · enter sends it to GitHub · esc cancels", c.file.Path, c.lines())
}

type prComposedMsg struct {
	seq  int
	text string
	err  error
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// lineComment is "42 text", "42: text" or "100-120 text".
var lineComment = regexp.MustCompile(`^\s*(\d+)(?:\s*-\s*(\d+))?\s*[: ]\s*(.+)$`)

// composeReply starts a reply in a thread.
func (m *model) composeReply(file prFile, t prThread) tea.Cmd {
	if len(t.Comments) == 0 {
		return nil
	}
	return m.beginCompose(&composeState{kind: "reply", thread: t, file: file,
		hint: fmt.Sprintf("reply to @%s in %s · enter sends it to GitHub · esc cancels", t.Comments[0].Author, file.Path)})
}

// composeLine starts a comment on one line of the new file.
func (m *model) composeLine(file prFile, line int) tea.Cmd {
	c := &composeState{kind: "line", file: file, line: line, start: line, anchor: line}
	c.setHint()
	return m.beginCompose(c)
}

func (m *model) composeGeneral() tea.Cmd {
	return m.beginCompose(&composeState{kind: "general", hint: "new comment on the PR · enter sends it · esc cancels"})
}

func (m *model) beginCompose(c *composeState) tea.Cmd {
	m.picker.compose = c
	m.setOmni("")
	return m.omni.Focus()
}

// startCompose begins a reply to the selected thread (ctrl+r) or a new
// comment on the selected file, thread's file or the PR itself (ctrl+n).
func (m *model) startCompose(kind string) tea.Cmd {
	p := m.picker
	if p.pr == nil || len(p.items) == 0 {
		return nil
	}
	value := p.items[p.sel].value
	var file *prFile
	var thread *prThread
	if a, b, ok := strings.Cut(value, ":"); ok {
		fi, _ := strconv.Atoi(a)
		ti, _ := strconv.Atoi(b)
		if fi < len(p.pr.Files) && ti < len(p.pr.Files[fi].Threads) {
			file, thread = &p.pr.Files[fi], &p.pr.Files[fi].Threads[ti]
		}
	} else if value != "" {
		for i := range p.pr.Files {
			if p.pr.Files[i].Path == value {
				file = &p.pr.Files[i]
			}
		}
	}
	switch {
	case kind == "reply" && thread != nil:
		return m.composeReply(*file, *thread)
	case kind == "reply":
		m.flash("to reply, select a thread in the Comments list, or click Reply on it")
		return nil
	case file != nil:
		return m.beginCompose(&composeState{kind: "line", file: *file,
			hint: fmt.Sprintf("new comment on %s · type `<line> <text>` or `<from>-<to> <text>`, or click a line (and another to extend) · enter sends it · esc cancels", file.Path)})
	}
	return m.composeGeneral()
}

// previewClick handles a click in the PR preview: buttons on a thread, a diff
// line (a comment on it), or the New comment button.
func (m *model) previewClick(x, y int) tea.Cmd {
	p := m.picker
	if p.pr == nil {
		return nil
	}
	rel := x - (m.pickerListWidth() + 1)
	row := p.pscroll + y - headerRows
	for _, h := range p.hits[row] {
		if rel < h.x0 || rel >= h.x1 {
			continue
		}
		if c := p.compose; c != nil {
			// While a line comment is open, clicking another line of the same file
			// stretches it from the first line clicked to this one.
			if c.kind == "line" && c.line != 0 && h.kind == "line" && p.pr.Files[h.file].Path == c.file.Path {
				c.start, c.line = min(c.anchor, h.line), max(c.anchor, h.line)
				c.setHint()
			}
			return nil
		}
		switch h.kind {
		case "general":
			return m.composeGeneral()
		case "line":
			return m.composeLine(p.pr.Files[h.file], h.line)
		case "reply":
			return m.composeReply(p.pr.Files[h.file], p.pr.Files[h.file].Threads[h.thread])
		case "resolve":
			return m.toggleThread(h.file, h.thread)
		}
	}
	return nil
}

type prResolvedMsg struct {
	thread   string
	resolved bool
	err      error
}

// toggleThread resolves or reopens a thread. The button flips at once and
// flips back if GitHub refuses.
func (m *model) toggleThread(fi, ti int) tea.Cmd {
	p := m.picker
	t := &p.pr.Files[fi].Threads[ti]
	if t.ID == "" {
		return nil
	}
	target := !t.Resolved
	t.Resolved = target
	p.pr.At = time.Now().UnixNano() // drops the cached previews
	m.rebuildPicker()
	dir, id := m.repo.Root, t.ID
	query := `mutation($id:ID!){unresolveReviewThread(input:{threadId:$id}){thread{id}}}`
	if target {
		query = `mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{id}}}`
	}
	return func() tea.Msg {
		_, err := ghGraphQL(dir, query, map[string]any{"id": id})
		return prResolvedMsg{id, target, err}
	}
}

func (m *model) onPRResolved(msg prResolvedMsg) {
	p := m.picker
	if p == nil || p.kind != "prdiff" || p.pr == nil || msg.err == nil {
		return
	}
	for fi := range p.pr.Files {
		for ti := range p.pr.Files[fi].Threads {
			if t := &p.pr.Files[fi].Threads[ti]; t.ID == msg.thread {
				t.Resolved = !msg.resolved
			}
		}
	}
	p.pr.At = time.Now().UnixNano()
	m.rebuildPicker()
	m.setErr(msg.err.Error())
}

// composeKey drives the omnibox while a comment is being written.
func (m *model) composeKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picker
	switch msg.String() {
	case "esc", "ctrl+c":
		p.compose = nil
		m.setOmni("")
		m.rebuildPicker()
		return nil
	case "enter":
		return m.sendCompose()
	}
	var cmd tea.Cmd
	m.omni, cmd = m.omni.Update(msg)
	return cmd
}

func (m *model) sendCompose() tea.Cmd {
	p, c := m.picker, m.picker.compose
	text := strings.TrimSpace(m.omni.Value())
	if text == "" {
		return nil
	}
	d := p.pr
	dir := m.repo.Root
	var post func() error
	switch c.kind {
	case "reply":
		id := c.thread.Comments[0].ID
		post = func() error {
			_, err := ghOutput(dir, "api", "--method", "POST", fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", d.Repo, d.Number, id), "-f", "body="+text)
			return err
		}
	case "line":
		start, line, comment := c.start, c.line, text
		if line == 0 {
			mt := lineComment.FindStringSubmatch(text)
			if mt == nil {
				m.setErr("start with the line number, like `42 text` or `100-120 text`")
				return nil
			}
			start, line, comment = atoi(mt[1]), atoi(mt[1]), strings.TrimSpace(mt[3])
			if mt[2] != "" {
				line = atoi(mt[2])
			}
		}
		if !c.file.rangeOK(start, line) {
			m.setErr(fmt.Sprintf("lines %d–%d are not one stretch of the diff of %s (a comment range must stay inside one hunk)", start, line, c.file.Path))
			return nil
		}
		review := map[string]any{"body": comment, "commit_id": d.HeadSHA, "path": c.file.Path, "line": line, "side": "RIGHT"}
		if start < line {
			review["start_line"], review["start_side"] = start, "RIGHT"
		}
		body, _ := json.Marshal(review)
		post = func() error {
			_, err := ghInput(dir, body, "api", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/comments", d.Repo, d.Number))
			return err
		}
	default:
		post = func() error {
			_, err := ghOutput(dir, "api", "--method", "POST", fmt.Sprintf("repos/%s/issues/%d/comments", d.Repo, d.Number), "-f", "body="+text)
			return err
		}
	}
	p.compose = nil
	m.setOmni("")
	prSeq++
	p.seq = prSeq
	seq := p.seq
	m.flash("sending to GitHub…")
	return func() tea.Msg { return prComposedMsg{seq, "sent to GitHub", post()} }
}

// onPRComposed reloads the PR so the new comment shows up.
func (m *model) onPRComposed(msg prComposedMsg) tea.Cmd {
	p := m.picker
	if p == nil || p.kind != "prdiff" || p.seq != msg.seq || p.pr == nil {
		return nil
	}
	if msg.err != nil {
		m.setErr(msg.err.Error())
		return nil
	}
	m.flash(msg.text)
	prSeq++
	p.seq = prSeq
	return reloadPR(m.repo.Root, p.pr.URL, p.seq)
}

func reloadPR(dir, url string, seq int) tea.Cmd {
	return func() tea.Msg {
		d, err := fetchPR(dir, url)
		return prDiffMsg{seq, d, err}
	}
}
