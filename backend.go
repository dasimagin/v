package main

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// backend is a coding agent v drives: Claude Code or Codex. Each runs as a
// subprocess signed in with the user's own subscription, and reports what
// happens as ev values.
type backend interface {
	Name() string
	Prompt(text string) error
	Interrupt() error
	// Answer resolves a permission request. reason goes to the agent with a
	// rejection.
	Answer(p *permission, allow bool, reason string) error
	// Steer adds a user message to the running turn; errNoSteer if the
	// agent can't take one mid-turn.
	Steer(text string) error
	Close()
	wait() tea.Cmd
}

var errNoSteer = errors.New("this agent can't take messages mid-turn")

type evKind int

const (
	evReady         evKind = iota // connected: model, session
	evTextStart                   // a reply starts
	evText                        // more reply text
	evThinkingStart               // a thinking block starts
	evThinking                    // more thinking text
	evTool                        // a tool call: text is "Name argument"
	evToolError                   // a tool failed: text is the first line
	evAsk                         // a permission request: perm
	evDone                        // the turn ended: secs, turns, or isErr with text
)

// ev is one thing that happened in the agent, the same for every backend.
type ev struct {
	kind    evKind
	text    string
	model   string
	session string
	perm    *permission
	secs    float64
	turns   int
	isErr   bool
}

// agentEvent carries one ev to Update; a tells stale agents apart.
type agentEvent struct {
	a backend
	e ev
}

// agentClosedMsg reports that the agent process exited.
type agentClosedMsg struct {
	a      backend
	stderr string
}

const denyMessage = "The user denied this in v."

// permission is a pending tool call the user has to answer.
type permission struct {
	id      any    // the backend's request id
	summary string // "Edit main.go", "Bash go test ./..."
	tool    string // the tool's name: "Edit", "Bash", "mcp__v__pr_reply"
	// propose builds the file as the tool call would leave it; nil for tool
	// calls that are not file edits.
	propose func() (*Doc, error)
	input   map[string]any // Claude: the tool input, echoed back on allow

	logIdx int    // the msgAsk entry in the chat log
	note   string // typed before choosing Edit; goes with the user's version
}

// waitFor returns a command that delivers the next ev of a to Update.
func waitFor(a backend, events <-chan ev, stderr func() string) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-events
		if !ok {
			return agentClosedMsg{a, stderr()}
		}
		return agentEvent{a, e}
	}
}
