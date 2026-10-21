package main

import (
	"fmt"
	"strings"
)

// A run is /review or /apply on its own screen: a separate conversation with
// its own agent, shown where the chat is. Esc leaves it and the chat you were
// in comes back as it was.
type runScreen struct {
	kind   string // "review" or "apply"
	title  string
	auto   bool
	rv     *reviewFlow // the run's tools and proposals; kept until the screen closes
	tools  []mcpTool   // what the run's agent can call
	system string      // added to its system prompt
	stash  chatState   // the conversation to return to
}

// chatState is what makes up one conversation with an agent.
type chatState struct {
	log        []message
	sessionID  string
	resumeNext bool
	queue      []string
}

// enterRun puts the chat aside and starts an empty conversation for a run.
// The chat's agent is stopped: its session is kept, and it resumes on the next
// prompt after the run.
func (m *model) enterRun(kind, title string, auto bool) bool {
	switch {
	case m.runScr != nil:
		m.setErr("a run is already open: leave it with esc first")
		return false
	case m.busy || m.perm != nil:
		m.setErr("wait for the agent to finish first")
		return false
	}
	m.showChat()
	stash := chatState{log: m.log, sessionID: m.sessionID, resumeNext: m.resumeNext, queue: m.queue}
	m.stopAgent()
	m.log, m.sessionID, m.resumeNext, m.queue = nil, "", false, nil
	m.interrupted, m.noting, m.choice = false, false, 0
	m.runScr = &runScreen{kind: kind, title: title, auto: auto, stash: stash}
	m.omni.SetValue("")
	return true
}

// leaveRun ends the run and brings the chat back.
func (m *model) leaveRun() {
	r := m.runScr
	if r == nil {
		return
	}
	if rv := m.rv; rv != nil { // still going: stop it
		rv.dropped = true
		if rv.cancel != nil {
			rv.cancel()
		}
		m.rv = nil
	}
	m.hub.set(nil)
	m.toolsOn = false
	m.stopAgent()
	m.runScr = nil
	m.log, m.sessionID, m.resumeNext, m.queue = r.stash.log, r.stash.sessionID, r.stash.sessionID != "" || r.stash.resumeNext, r.stash.queue
	m.interrupted, m.noting, m.choice = false, false, 0
	if r.kind == "apply" {
		notice := fmt.Sprintf("A separate /apply run, with a fresh agent, just worked in %s: it may have edited files, made commits, pushed, and replied in the PR's review threads. Read the git state before continuing.", m.workdir)
		if m.workspaceNotice != "" {
			notice = m.workspaceNotice + " " + notice
		}
		m.workspaceNotice = notice
	}
	m.note("back from the " + r.kind + " run")
	m.omni.SetValue("")
	m.omni.Focus()
}

// chatHeader is the header of the chat area: plain for the chat, titled for a run.
func (m *model) chatHeader() []string {
	r := m.runScr
	if r == nil {
		return m.header("", "", "")
	}
	info := "done · esc to leave"
	switch {
	case m.rv.waiting():
		info = "waiting for your answer"
	case m.rv != nil || m.busy:
		info = "working · esc stops it"
	}
	mode := "manual"
	if r.auto {
		mode = "auto"
	}
	return m.header(fgStyle.Bold(true).Render(strings.Title(r.kind)+" · "+r.title), mode+" · experimental · "+info, "")
}

// runBlocked are the commands that would pull the rug from under a run.
var runBlocked = map[string]bool{"reset": true, "resume": true, "workdir": true, "switch": true, "branch": true, "apply": true, "review": true}
