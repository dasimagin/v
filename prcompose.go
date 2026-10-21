package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// composeState is a comment being written on the PR screen: the omnibox holds
// its text until Enter sends it or Esc drops it.
type composeState struct {
	kind   string // "reply", "line" or "general"
	thread prThread
	file   prFile
	hint   string
}

type prComposedMsg struct {
	seq  int
	text string
	err  error
}

var lineComment = regexp.MustCompile(`^\s*(\d+)\s*[: ]\s*(.+)$`)

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
	c := &composeState{}
	switch {
	case kind == "reply" && thread != nil && len(thread.Comments) > 0:
		c.kind, c.thread, c.file = "reply", *thread, *file
		c.hint = fmt.Sprintf("reply to @%s in %s · enter sends it to GitHub · esc cancels", thread.Comments[0].Author, file.Path)
	case kind == "reply":
		m.flash("to reply, switch the list to Comments (ctrl+l) and select a thread")
		return nil
	case file != nil:
		c.kind, c.file = "line", *file
		c.hint = fmt.Sprintf("new comment on %s · type `<line> <text>`, the line numbers are in the diff · enter sends it · esc cancels", file.Path)
	default:
		c.kind = "general"
		c.hint = "new comment on the PR · enter sends it · esc cancels"
	}
	p.compose = c
	m.setOmni("")
	return m.omni.Focus()
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
		mt := lineComment.FindStringSubmatch(text)
		if mt == nil {
			m.setErr("start with the line number, like `42 text`")
			return nil
		}
		line, _ := strconv.Atoi(mt[1])
		if !c.file.newLines()[line] {
			m.setErr(fmt.Sprintf("line %d is not in the diff of %s", line, c.file.Path))
			return nil
		}
		body, _ := json.Marshal(map[string]any{"body": strings.TrimSpace(mt[2]), "commit_id": d.HeadSHA, "path": c.file.Path, "line": line, "side": "RIGHT"})
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
