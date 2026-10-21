package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// toolSummary is a one-line description of a tool call, e.g. "Edit main.go".
func toolSummary(name string, in map[string]any, dirs ...string) string {
	for _, k := range []string{"file_path", "command", "pattern", "path", "url", "description"} {
		if v, ok := in[k].(string); ok && v != "" {
			if k == "file_path" || k == "path" {
				v = shortPath(v, dirs...)
			}
			v = strings.SplitN(v, "\n", 2)[0]
			return name + " " + v
		}
	}
	return name
}

// fromUser reports whether a tool error is v's own answer to a permission
// request; those are already shown on the question line.
func fromUser(line string) bool {
	return line == denyMessage || strings.HasPrefix(line, "The user rejected this") ||
		strings.HasPrefix(line, "The user edited your proposed change")
}

// shortPath shows a path relative to the working directory when it is inside it.
func shortPath(p string, dirs ...string) string {
	wd, _ := os.Getwd()
	if len(dirs) > 0 {
		wd = dirs[0]
	}
	p = workPath(wd, p)
	if rel, err := filepath.Rel(wd, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return tildify(p)
}

// onEvent turns one agent event into chat log entries and state.
func (m *model) onEvent(e ev) {
	defer m.sendQueued()
	last := func(k msgKind) *message {
		if n := len(m.log); n > 0 && m.log[n-1].kind == k {
			return &m.log[n-1]
		}
		return nil
	}
	switch e.kind {
	case evReady:
		if !m.agentReady {
			m.agentReady, m.sessionID = true, e.session
			m.note("connected to " + m.agent.Name() + " · " + e.model)
		}
	case evTextStart:
		m.log = append(m.log, message{kind: msgAgent})
	case evThinkingStart:
		m.log = append(m.log, message{kind: msgThinking})
	case evText:
		if l := last(msgAgent); l != nil {
			l.text += e.text
		}
	case evThinking:
		if l := last(msgThinking); l != nil {
			l.text += e.text
		}
	case evTool:
		m.log = append(m.log, message{kind: msgTool, text: e.text})
	case evToolError:
		m.noteErr(e.text)
	case evAsk:
		if !m.runPermission(e.perm) {
			m.ask(e.perm)
		}
	case evDone:
		m.finishRunTurn()
		m.busy = false
		switch {
		case m.interrupted:
			m.interrupted = false // the "interrupted" note is already there
		case e.isErr:
			m.noteErr(e.text)
		case e.turns > 0:
			m.note(fmt.Sprintf("done · %.1fs · %d turns", e.secs, e.turns))
		default:
			m.note(fmt.Sprintf("done · %.1fs", e.secs))
		}
	}
}

// ask shows a permission request: the tool call line turns into the
// question (and later into its answer), and file edits open as a diff.
func (m *model) ask(p *permission) {
	m.perm, m.choice = p, 0
	m.omni.Blur() // keys answer the question until Tab opens a note
	p.logIdx = -1
	for i := len(m.log) - 1; i >= 0 && p.logIdx < 0; i-- {
		if m.log[i].kind == msgTool && m.log[i].text == p.summary {
			m.log[i].kind, p.logIdx = msgAsk, i
		}
	}
	if p.logIdx < 0 {
		p.logIdx = len(m.log)
		m.log = append(m.log, message{kind: msgAsk, text: p.summary})
	}
	if p.propose != nil {
		m.openReview(p)
	}
}

// answerPermission resolves the pending tool request. note, if any, goes
// with the answer: as the reason of a rejection, in the message about the
// user's version, or as a message right after an approval. A proposal the
// user edited is saved by v and reported to the agent with their changes.
func (m *model) answerPermission(allow bool, note string) {
	p := m.perm
	edited := allow && m.mode == modeReview && m.doc.Ed.Dirty
	if note == "" {
		note = p.note
	}
	reason := denyMessage
	if note != "" {
		reason = "The user rejected this and said: " + note
	}
	if edited {
		msg, err := applyUserVersion(m.doc)
		if err != nil {
			m.setErr("save: " + err.Error()) // still waiting for an answer
			return
		}
		if note != "" {
			msg += "\nThe user also said: " + note
		}
		allow, reason = false, msg
	}
	m.perm = nil
	if err := m.agent.Answer(p, allow, reason); err != nil {
		m.setErr(err.Error())
	}
	answer := "n"
	switch {
	case edited:
		answer = "e"
	case allow:
		answer = "y"
	}
	if p.logIdx < len(m.log) {
		m.log[p.logIdx].answer = answer
	}
	if note != "" {
		m.log = append(m.log, message{kind: msgUser, text: note})
		if allow {
			// An approval carries no text: the note goes into the running
			// turn if the agent takes that, or as the next prompt.
			if err := m.agent.Steer(note); errors.Is(err, errNoSteer) {
				m.queue = append(m.queue, note)
			}
		}
	}
	if m.mode == modeReview {
		m.pop()
	}
	m.omni.Focus()
}

// prompt sends text to the agent, starting it on first use.
func (m *model) prompt(text string) tea.Cmd {
	if m.busy {
		m.queue = append(m.queue, text)
		m.note("  ⎿ queued: sent when the agent finishes")
		return nil
	}
	cmd := m.connect()
	if m.agent == nil {
		return nil
	}
	if m.workspaceNotice != "" {
		text = "[Workspace context: " + m.workspaceNotice + "]\n\n" + text
	}
	if err := m.agent.Prompt(text); err != nil {
		m.note(err.Error())
		return cmd
	}
	m.workspaceNotice = ""
	m.busy = true
	return cmd
}

// connect starts the agent chosen in the settings if it is not running and
// returns the command that listens to it.
func (m *model) connect() tea.Cmd {
	resume := ""
	if m.agent != nil && m.agentStale && !m.busy {
		// New model, effort or workdir: reconnect on the same session.
		resume = m.sessionID
		m.stopAgent()
	}
	if m.agent != nil {
		return nil
	}
	if m.resumeNext {
		resume = m.sessionID
	}
	// A run's agent is always Claude Code, with the run's tools and rules.
	var a backend
	var err error
	var tools []mcpTool
	if r := m.runScr; r != nil {
		tools = r.tools
		a, err = startClaude(m.workdir, m.settings, resume, tools, r.system)
	} else {
		a, err = startBackend(m.workdir, m.settings, resume, nil)
	}
	if err != nil {
		m.resumeNext = resume != ""
		m.noteErr(err.Error())
		return nil
	}
	m.agent, m.resumeNext, m.agentHasTools = a, false, len(tools) > 0
	return a.wait()
}

// startBackend starts the agent named in the settings.
func startBackend(dir string, s Settings, resume string, tools []mcpTool) (backend, error) {
	if s.Agent == agentCodex {
		return startCodex(dir, s, resume)
	}
	return startClaude(dir, s, resume, tools, "")
}

// sendQueued sends the next queued prompt once the agent is idle.
func (m *model) sendQueued() {
	if m.busy || m.agent == nil || len(m.queue) == 0 {
		return
	}
	text := m.queue[0]
	m.queue = m.queue[1:]
	if err := m.agent.Prompt(text); err != nil {
		m.noteErr(err.Error())
		return
	}
	m.busy = true
}

// agentSettingsChanged applies a new model or effort from the next message.
// Claude Code restarts on the same session for it; Codex takes them per turn.
func (m *model) agentSettingsChanged() {
	if m.agent != nil && m.settings.Agent != agentCodex {
		m.agentStale = true
	}
	m.flash(m.agentLabel() + " from the next message")
}

// agentChanged switches between Claude Code and Codex: the session ends and
// the next prompt starts the other agent.
func (m *model) agentChanged() {
	m.stopAgent()
	m.sessionID, m.resumeNext = "", false
	m.flash(m.agentLabel() + " from the next message, in a new session")
}

// agentLabel names the agent, model and effort: "Opus 5.5 · high",
// "Codex · GPT-6.1-Sol · high".
func (m *model) agentLabel() string {
	model, effort := m.agentParts()
	return model + " · " + effort
}

// agentParts are the model and effort labels, each a dropdown in the status line.
func (m *model) agentParts() (model, effort string) {
	s := m.settings
	if s.Agent != agentCodex {
		return modelName(s.Model), s.Effort
	}
	id, effort := s.CodexModel, s.CodexEffort
	if id == "" {
		id = codexDefault().ID
	}
	model = "Codex"
	if id != "" {
		model += " · " + codexModelName(id)
	}
	if effort == "" {
		effort = "default"
	}
	return model, effort
}

// stopAgent ends the agent session, if any.
func (m *model) stopAgent() {
	if m.agent != nil {
		m.agent.Close()
	}
	m.agent, m.busy, m.perm, m.agentReady, m.agentStale, m.agentHasTools = nil, false, nil, false, false, false
	m.queue = nil
}

// choice is one answer to a pending tool call.
type choice struct{ key, label string }

// choices are the answers for the pending tool call: edits under review can
// also be edited before accepting.
func (m *model) choices() []choice {
	if m.perm == nil && m.reviewAsking() {
		return []choice{{"y", "Yes, post it"}, {"n", "No, skip it"}, {"a", "Yes, and accept the rest"}, {"x", "Stop the review"}}
	}
	if m.mode == modeReview && m.perm.propose != nil {
		return []choice{{"y", "Yes"}, {"e", "Edit first"}, {"n", "No"}}
	}
	return []choice{{"y", "Yes"}, {"n", "No"}}
}

// showChoices reports whether the choice list replaces the status line.
func (m *model) showChoices() bool {
	return (m.perm != nil && (!m.fileMode() || !m.doc.Ed.On)) || (m.perm == nil && m.reviewAsking())
}

// reviewAsking reports whether a review comment is waiting for an answer here.
func (m *model) reviewAsking() bool { return m.rv.waiting() && m.mode == modeChat }

func (m *model) choose(key, note string) {
	if m.perm == nil {
		m.reviewChoose(key, note)
		return
	}
	switch key {
	case "y":
		m.answerPermission(true, note)
	case "n":
		m.answerPermission(false, note)
	case "e":
		m.perm.note = note // sent with the user's version
		m.startEdit(-1)
	}
}

// renderChoices draws the question and the answers under the omnibox.
func (m *model) renderChoices() []string {
	ask := bg.Foreground(lipgloss.Color(cur.Type)).Bold(true)
	question := "? Post this comment to the PR?"
	if m.perm != nil {
		question = "? Allow " + m.perm.summary + "?"
	} else if q := m.rv.ask.card.Question; q != "" {
		question = "? " + q
	}
	out := []string{bg.Render(" ") + ask.Render(question)}
	labelW := 0
	for _, c := range m.choices() {
		labelW = max(labelW, len(c.label))
	}
	for i, c := range m.choices() {
		mark, st := "    ", fgStyle
		if i == m.choice {
			mark, st = "  › ", accentStyle.Bold(true)
		}
		out = append(out, st.Render(mark+c.label)+blank(labelW-len(c.label)+3)+hintStyle.Render(c.key))
	}
	for i, l := range out {
		out[i] = l + blank(m.w-lipgloss.Width(l))
	}
	return out
}
