package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// uiEvent is something the agent did, as the page gets it.
type uiEvent struct {
	Kind    string  `json:"kind"` // ready, text-start, text, thinking-start, thinking, tool, tool-error, ask, done, closed
	Text    string  `json:"text,omitempty"`
	ID      string  `json:"id,omitempty"` // ask: the request to answer
	Tool    string  `json:"tool,omitempty"`
	Model   string  `json:"model,omitempty"`
	Secs    float64 `json:"secs,omitempty"`
	Turns   int     `json:"turns,omitempty"`
	IsError bool    `json:"isError,omitempty"`
	ToolID  string  `json:"toolId,omitempty"` // tool, tool-result: the call
	Detail  string  `json:"detail,omitempty"` // tool: its arguments, readable
}

// uiMsg is a line of a past conversation.
type uiMsg struct {
	Kind string `json:"kind"` // user, agent, tool, thinking
	Text string `json:"text"`
}

// uiChat is the agent session of the graphical UI. It starts Claude Code on
// the first prompt and reports what it does through emit.
type uiChat struct {
	emit func(uiEvent)

	mu        sync.Mutex
	agent     backend
	started   string // model and effort the agent runs with
	sessionID string
	resume    bool // the next start continues sessionID
	perms     map[string]*permission
}

func newUIChat(emit func(uiEvent)) *uiChat {
	return &uiChat{emit: emit, perms: map[string]*permission{}}
}

// Send gives text to the agent, starting (or restarting on a new model or
// effort) it first.
func (c *uiChat) Send(dir, text string) error {
	s, _ := loadSettings()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agent != nil && c.started != s.Model+"/"+s.Effort {
		c.stop()
		c.resume = c.sessionID != ""
	}
	if c.agent == nil {
		resume := ""
		if c.resume {
			resume = c.sessionID
		}
		a, err := startClaude(dir, s, resume, nil, "")
		if err != nil {
			return err
		}
		c.agent, c.started, c.resume = a, s.Model+"/"+s.Effort, false
		go c.pump(a)
	}
	return c.agent.Prompt(text)
}

// pump turns the agent's events into page events until it exits.
func (c *uiChat) pump(a *claudeAgent) {
	for e := range a.events {
		switch e.kind {
		case evReady:
			c.mu.Lock()
			c.sessionID = e.session
			c.mu.Unlock()
			c.emit(uiEvent{Kind: "ready", Model: e.model})
		case evTextStart:
			c.emit(uiEvent{Kind: "text-start"})
		case evText:
			c.emit(uiEvent{Kind: "text", Text: e.text})
		case evThinkingStart:
			c.emit(uiEvent{Kind: "thinking-start"})
		case evThinking:
			c.emit(uiEvent{Kind: "thinking", Text: e.text})
		case evTool:
			c.emit(uiEvent{Kind: "tool", Text: e.text, Tool: e.tool, ToolID: e.toolID, Detail: toolDetail(e.tool, e.input)})
		case evToolResult:
			c.emit(uiEvent{Kind: "tool-result", ToolID: e.toolID, Text: clipText(e.text, uiResultMax), IsError: e.isErr})
		case evToolError:
			c.emit(uiEvent{Kind: "tool-error", Text: e.text})
		case evAsk:
			// In auto mode the agent goes on by itself, except for what autoDeny refuses.
			if s, _ := loadSettings(); s.Mode == reviewAuto {
				if reason := autoDeny(e.perm.tool, e.perm.input); reason != "" {
					_ = a.Answer(e.perm, false, reason)
					c.emit(uiEvent{Kind: "note", Text: "denied: " + reason})
				} else {
					_ = a.Answer(e.perm, true, "")
				}
				continue
			}
			id := fmt.Sprint(e.perm.id)
			c.mu.Lock()
			c.perms[id] = e.perm
			c.mu.Unlock()
			c.emit(uiEvent{Kind: "ask", ID: id, Text: e.perm.summary, Tool: e.perm.tool})
		case evDone:
			c.emit(uiEvent{Kind: "done", Text: e.text, Secs: e.secs, Turns: e.turns, IsError: e.isErr})
		}
	}
	c.mu.Lock()
	if c.agent == backend(a) {
		c.agent = nil
	}
	c.mu.Unlock()
	c.emit(uiEvent{Kind: "closed", Text: strings.TrimSpace(a.stderr.String())})
}

// Answer allows or denies a tool call the agent asked about.
func (c *uiChat) Answer(id string, allow bool) error {
	c.mu.Lock()
	p, a := c.perms[id], c.agent
	delete(c.perms, id)
	c.mu.Unlock()
	if p == nil || a == nil {
		return fmt.Errorf("the request is gone")
	}
	return a.Answer(p, allow, denyMessage)
}

// Interrupt stops the turn that is running.
func (c *uiChat) Interrupt() error {
	c.mu.Lock()
	a := c.agent
	c.mu.Unlock()
	if a == nil {
		return nil
	}
	return a.Interrupt()
}

// stop ends the agent; the caller holds c.mu.
func (c *uiChat) stop() {
	if c.agent != nil {
		c.agent.Close()
	}
	c.agent = nil
	c.perms = map[string]*permission{}
}

// Reset ends the session and forgets it.
func (c *uiChat) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop()
	c.sessionID, c.resume = "", false
}

// Resume makes a past session the current one: the next prompt continues it.
// It returns the conversation so far.
func (c *uiChat) Resume(dir, path string) []uiMsg {
	c.mu.Lock()
	c.stop()
	c.sessionID, c.resume = strings.TrimSuffix(filepath.Base(path), ".jsonl"), true
	c.mu.Unlock()
	out := []uiMsg{}
	for _, m := range sessionLog(path, dir) {
		var kind string
		switch m.kind {
		case msgUser:
			kind = "user"
		case msgAgent:
			kind = "agent"
		case msgTool:
			kind = "tool"
		case msgThinking:
			kind = "thinking"
		default:
			continue
		}
		out = append(out, uiMsg{kind, m.text})
	}
	return out
}

const (
	uiDetailMax = 6 << 10
	uiResultMax = 20 << 10
)

// clipText keeps the start of s, up to max bytes, on a character boundary.
func clipText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "\n… cut"
}

// resultText is the text of a tool result: a string, or a list of text blocks.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var out []string
	for _, p := range parts {
		out = append(out, p.Text)
	}
	return strings.Join(out, "\n")
}

// diffText shows a replacement as lines to remove and lines to add.
func diffText(oldText, newText string) string {
	var b strings.Builder
	for _, l := range strings.Split(oldText, "\n") {
		b.WriteString("- " + l + "\n")
	}
	for _, l := range strings.Split(newText, "\n") {
		b.WriteString("+ " + l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolDetail is what a tool call was given, in a form to read under its summary.
func toolDetail(name string, in map[string]any) string {
	str := func(k string) string { s, _ := in[k].(string); return s }
	var out string
	switch name {
	case "Bash":
		out = str("command")
	case "Edit":
		out = diffText(str("old_string"), str("new_string"))
	case "MultiEdit":
		edits, _ := in["edits"].([]any)
		var parts []string
		for _, e := range edits {
			if m, ok := e.(map[string]any); ok {
				o, _ := m["old_string"].(string)
				n, _ := m["new_string"].(string)
				parts = append(parts, diffText(o, n))
			}
		}
		out = strings.Join(parts, "\n")
	case "Write":
		out = str("content")
	default:
		keys := make([]string, 0, len(in))
		for k := range in {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var lines []string
		for _, k := range keys {
			switch v := in[k].(type) {
			case string:
				lines = append(lines, k+": "+v)
			case float64, bool:
				lines = append(lines, fmt.Sprintf("%s: %v", k, v))
			default:
				b, _ := json.Marshal(v)
				lines = append(lines, k+": "+string(b))
			}
		}
		out = strings.Join(lines, "\n")
	}
	return clipText(out, uiDetailMax)
}
