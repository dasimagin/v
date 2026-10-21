package main

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// applyRef is a review thread together with the file it sits in.
type applyRef struct {
	file   prFile
	thread prThread
}

// applyPrompt is the task of the agent that applies review feedback: the
// instructions of prompts/fixer.md, then the threads. The threads are other
// people's text, so the instructions say to treat them as feedback about the
// code and nothing else.
func applyPrompt(workdir string, d *prData, refs []applyRef, branch string, auto, loop bool, round, rounds int) (text, warn string) {
	head, warn := renderPrompt("fixer", workdir, promptData{Number: d.Number, Title: d.Title, Head: d.Head, Base: d.Base, Branch: branch, Auto: auto, Loop: loop, Round: round, MaxRounds: rounds})
	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n\n## The threads\n")
	for i, r := range refs {
		t := r.thread
		where := fmt.Sprintf("%s:%d", r.file.Path, t.Line)
		switch {
		case t.Line == 0:
			where = r.file.Path + " (the line it was on has changed)"
		case t.Start > 0 && t.Start < t.Line:
			where = fmt.Sprintf("%s:%d-%d", r.file.Path, t.Start, t.Line)
		}
		fmt.Fprintf(&b, "\nThread %d · id %s · %s\n", i+1, t.ID, where)
		for _, c := range t.Comments {
			fmt.Fprintf(&b, "@%s: %s\n", c.Author, strings.TrimSpace(c.Body))
		}
		if t.Line > 0 {
			if hunk, _ := r.file.excerpt(t.Line); len(hunk) > 0 {
				b.WriteString("The diff around it:\n```diff\n" + strings.Join(hunk, "\n") + "\n```\n")
			}
		}
	}
	return b.String(), warn
}

// unresolvedThreads are the open threads of the PR with something in them.
func (d *prData) unresolvedThreads() []applyRef {
	var refs []applyRef
	for _, f := range d.Files {
		for _, t := range f.Threads {
			if !t.Resolved && len(t.Comments) > 0 {
				refs = append(refs, applyRef{f, t})
			}
		}
	}
	return refs
}

type applyReadyMsg struct {
	data *prData
	err  error
}

// reviewEndMsg releases the command that waits on a finished run's channel.
type reviewEndMsg struct{ flow *reviewFlow }

// cmdApply hands the open review threads of this branch's PR to the chat agent.
func (m *model) cmdApply(string) tea.Cmd {
	switch {
	case m.settings.Agent == agentCodex:
		m.setErr("/apply needs Claude Code: pick it as the agent in /config")
		return nil
	case m.rv != nil:
		m.setErr("a review or apply is already in progress")
		return nil
	case m.busy || m.perm != nil:
		m.setErr("wait for the agent to finish first")
		return nil
	case m.repo == nil || m.repo.Branch == "" || len(m.repo.pullRequests()) == 0:
		m.setErr("no PR for this branch · /pr <url> to link one")
		return nil
	}
	pr := m.repo.selectedPR()
	if repo, _ := parsePRURL(pr.URL); repo == "" {
		m.setErr("/apply reads review threads from a github.com PR")
		return nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		m.setErr("install gh and run gh auth login to read review threads")
		return nil
	}
	m.note("reading the review threads of " + prLabel(*pr) + "…")
	dir, url := m.repo.Root, pr.URL
	return func() tea.Msg {
		d, err := fetchPR(dir, url)
		return applyReadyMsg{d, err}
	}
}

func (m *model) onApplyReady(msg applyReadyMsg) tea.Cmd {
	if msg.err != nil {
		m.noteErr(msg.err.Error())
		return nil
	}
	refs := msg.data.unresolvedThreads()
	if len(refs) == 0 {
		m.note(fmt.Sprintf("no unresolved threads on PR #%d", msg.data.Number))
		return nil
	}
	return m.sendApply(msg.data, refs)
}

// applyThread hands one thread of the PR screen to the agent and returns to the chat.
func (m *model) applyThread(fi, ti int) tea.Cmd {
	p := m.picker
	if p == nil || p.pr == nil || fi >= len(p.pr.Files) || ti >= len(p.pr.Files[fi].Threads) {
		return nil
	}
	d, ref := p.pr, applyRef{p.pr.Files[fi], p.pr.Files[fi].Threads[ti]}
	m.closePicker(true)
	return m.sendApply(d, []applyRef{ref})
}

// sendApply starts an apply run: the chat agent gets v's pr_* tools (it is
// restarted on the same session if it was started without them), and the
// threads as its task.
func (m *model) sendApply(d *prData, refs []applyRef) tea.Cmd {
	switch {
	case m.rv != nil:
		m.setErr("a review or apply is already in progress")
		return nil
	case m.settings.Agent == agentCodex:
		m.setErr("/apply needs Claude Code: pick it as the agent in /config")
		return nil
	case m.busy || m.perm != nil:
		m.setErr("wait for the agent to finish first")
		return nil
	}
	if err := checkedOutAt(m.repo.Root, d); err != nil {
		m.noteErr(err.Error())
		return nil
	}
	auto := m.settings.ApplyMode == reviewAuto
	if !m.enterRun("apply", fmt.Sprintf("PR #%d", d.Number), auto) {
		return nil
	}
	// A fresh agent on its own screen: it starts empty, with v's pr_* tools.
	rv := &reviewFlow{apply: true, url: d.URL, repo: d.Repo, number: d.Number, dir: m.repo.Root, data: d,
		auto: auto, out: make(chan tea.Msg, 64), running: true}
	m.rv, m.runScr.rv = rv, rv
	m.hub.set(rv)
	m.toolsOn = true
	m.runScr.tools = rv.withNotes(applyTools(m.hub))
	noun := "threads"
	if len(refs) == 1 {
		noun = "thread"
	}
	mode := "asking about every step"
	if rv.auto {
		mode = "on its own"
	}
	m.note(fmt.Sprintf("handing %d review %s to a fresh agent, working %s · type to guide it, esc leaves", len(refs), noun, mode))
	task, warn := applyPrompt(m.repo.Root, d, refs, m.repo.Branch, rv.auto, false, 0, 0)
	if warn != "" {
		m.noteErr(warn)
	}
	cmd := m.prompt(task)
	m.armRun(rv)
	return tea.Batch(cmd, rv.wait())
}

// armRun points the run's cancel at the current agent.
func (m *model) armRun(rv *reviewFlow) {
	if a := m.agent; a != nil {
		rv.cancel = func() { _ = a.Interrupt() }
	}
}

// resumeRunTurn readies the run for another turn, started by a message the
// user types once the previous one is over: the agent's tools work again and
// its questions reach the user.
func (m *model) resumeRunTurn() tea.Cmd {
	r := m.runScr
	if r == nil || r.rv == nil || m.rv != nil {
		return nil
	}
	rv := r.rv
	rv.running, rv.dropped = true, false
	m.rv = rv
	return rv.wait()
}

// runPermission answers the agent's tool requests during a run. v's own tools
// gate themselves. A review only reads: read tools and read-only git are
// allowed, everything else refused. An apply run in auto mode may do anything
// except what autoDeny refuses; in manual mode its edits and commands go to
// the user as usual.
func (m *model) runPermission(p *permission) bool {
	rv := m.rv
	if rv == nil || m.agent == nil || m.runScr == nil {
		return false
	}
	deny := func(reason string) bool {
		m.noteErr("denied: " + reason)
		_ = m.agent.Answer(p, false, reason)
		return true
	}
	switch {
	case ownTool(p.tool):
	case !rv.apply: // a review
		cmd, _ := p.input["command"].(string)
		switch {
		case readTools[p.tool]:
		case p.tool == "Bash" && readOnlyGit(cmd):
		default:
			return deny("the reviewer reads the code; it does not edit files or run commands (read-only git is fine)")
		}
	case rv.auto:
		if reason := autoDeny(p.tool, p.input); reason != "" {
			return deny(reason)
		}
	default:
		return false
	}
	_ = m.agent.Answer(p, true, "")
	return true
}

// finishRunTurn ends the agent's turn in a run. The run itself goes on until
// you leave: type again to guide the agent further. A review publishes what
// the user accepted, and a note that never reached the agent becomes its next message.
func (m *model) finishRunTurn() {
	rv := m.rv
	if rv == nil || m.runScr == nil {
		return
	}
	m.rv = nil
	rv.running, rv.ask = false, nil
	select {
	case rv.out <- reviewEndMsg{rv}: // lets the waiting command return
	default:
	}
	if m.loop != nil { // the loop decides what follows
		m.deferred = m.loopTurnEnded(rv)
		rv.dropped = false
		return
	}
	var cmds []tea.Cmd
	if !rv.apply {
		cmds = append(cmds, m.finishReviewTurn(rv))
	}
	if rest := rv.takeNotes(); rest != "" && !rv.dropped {
		// delivered only with a tool result so far; the turn is over, so send it now
		cmds = append(cmds, m.resumeRunTurn(), m.prompt("Note from the user: "+rest))
	}
	rv.dropped = false
	m.deferred = tea.Batch(cmds...)
}
