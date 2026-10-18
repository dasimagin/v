package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// systemPrompt is appended to Claude Code's own system prompt.
const systemPrompt = `You are running inside v, a terminal UI for coding together with the user.
The user sees every edit you propose as a diff and approves or rejects it. Keep replies short.
The user may change your proposed edit before accepting it; the file then holds their version.
Read a file again before editing it a second time.`

// Agent is a long-running `claude -p` process speaking stream-json on stdin
// and stdout. It uses the user's Claude Code login: ANTHROPIC_API_KEY is
// removed from its environment so the subscription is used, not an API key.
type Agent struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	mu     sync.Mutex
	events chan agentMsg
	stderr bytes.Buffer
	reqN   int
}

// agentMsg is one line from the CLI; only the fields v uses are decoded.
type agentMsg struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Message   json.RawMessage `json:"message"`
	Event     json.RawMessage `json:"event"`
	Model     string          `json:"model"`
	Result    string          `json:"result"`
	IsError   bool            `json:"is_error"`
	Duration  int             `json:"duration_ms"`
	SessionID string          `json:"session_id"`
	NumTurns  int             `json:"num_turns"`
}

// agentEvent carries one message to Update; a tells stale agents apart.
type agentEvent struct {
	a   *Agent
	msg agentMsg
}

// agentClosedMsg reports that the process exited.
type agentClosedMsg struct {
	a      *Agent
	stderr string
}

// startAgent runs Claude Code in dir with the model and effort from s;
// resume, if set, continues that session.
func startAgent(dir string, s Settings, resume string) (*Agent, error) {
	model := s.Model
	if env := os.Getenv("V_CLAUDE_MODEL"); env != "" {
		model = env // only for testing v cheaply
	}
	cmd := exec.Command("claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-prompt-tool", "stdio",
		"--append-system-prompt", systemPrompt,
		"--model", model, "--effort", s.Effort,
		"--thinking-display", "summarized", // without it thinking blocks come empty
	)
	if resume != "" {
		cmd.Args = append(cmd.Args, "--resume", resume)
	}
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	a := &Agent{cmd: cmd, events: make(chan agentMsg, 256)}
	cmd.Stderr = &a.stderr
	var err error
	if a.in, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var m agentMsg
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				a.events <- m
			}
		}
		_ = cmd.Wait()
		close(a.events)
	}()
	return a, nil
}

// wait returns a command that delivers the next agent message to Update.
func (a *Agent) wait() tea.Cmd {
	return func() tea.Msg {
		m, ok := <-a.events
		if !ok {
			return agentClosedMsg{a, strings.TrimSpace(a.stderr.String())}
		}
		return agentEvent{a, m}
	}
}

func (a *Agent) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err = a.in.Write(append(b, '\n'))
	return err
}

// Prompt sends a user message.
func (a *Agent) Prompt(text string) error {
	return a.send(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": text},
		"parent_tool_use_id": nil,
		"session_id":         "default",
	})
}

// Interrupt stops the current turn.
func (a *Agent) Interrupt() error {
	a.mu.Lock()
	a.reqN++
	id := fmt.Sprintf("v_%d", a.reqN)
	a.mu.Unlock()
	return a.send(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "interrupt"}})
}

// Answer replies to a can_use_tool request.
func (a *Agent) Answer(requestID string, allow bool, input map[string]any, message string) error {
	body := map[string]any{"behavior": "deny", "message": message}
	if allow {
		body = map[string]any{"behavior": "allow", "updatedInput": input}
	}
	return a.send(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"subtype": "success", "request_id": requestID, "response": body},
	})
}

// Close ends the session; the process exits when its stdin closes.
func (a *Agent) Close() {
	_ = a.in.Close()
	if a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
}

const denyMessage = "The user denied this in v."

// permission is a pending can_use_tool request the user has to answer.
type permission struct {
	id     string
	tool   string
	input  map[string]any
	logIdx int    // the msgAsk entry in the chat log
	note   string // typed before choosing Edit; goes with the user's version
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
	ToolUseID string          `json:"tool_use_id"`
}

func blocks(raw json.RawMessage) []contentBlock {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var bs []contentBlock
	if json.Unmarshal(msg.Content, &bs) != nil {
		return nil
	}
	return bs
}

// toolSummary is a one-line description of a tool call, e.g. "Edit main.go".
func toolSummary(name string, in map[string]any) string {
	for _, k := range []string{"file_path", "command", "pattern", "path", "url", "description"} {
		if v, ok := in[k].(string); ok && v != "" {
			if k == "file_path" || k == "path" {
				v = shortPath(v)
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
func shortPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return tildify(p)
}

// firstLine of a tool result, for error notes.
func firstLine(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var parts []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(raw, &parts)
		for _, p := range parts {
			s += p.Text
		}
	}
	return strings.SplitN(strings.TrimSpace(s), "\n", 2)[0]
}

// onAgent turns one agent message into chat log entries and state.
func (m *model) onAgent(msg agentMsg) {
	defer m.sendQueued()
	switch msg.Type {
	case "system":
		if msg.Subtype == "init" && !m.agentReady {
			m.agentReady, m.sessionID = true, msg.SessionID
			m.note("connected to Claude Code · " + msg.Model)
		}
	case "stream_event":
		var ev struct {
			Type         string `json:"type"`
			ContentBlock struct {
				Type string `json:"type"`
			} `json:"content_block"`
			Delta struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"delta"`
		}
		if json.Unmarshal(msg.Event, &ev) != nil {
			return
		}
		last := func(k msgKind) *message {
			if n := len(m.log); n > 0 && m.log[n-1].kind == k {
				return &m.log[n-1]
			}
			return nil
		}
		switch {
		case ev.Type == "content_block_start" && ev.ContentBlock.Type == "text":
			m.log = append(m.log, message{kind: msgAgent})
		case ev.Type == "content_block_start" && ev.ContentBlock.Type == "thinking":
			m.log = append(m.log, message{kind: msgThinking})
		case ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta":
			if l := last(msgAgent); l != nil {
				l.text += ev.Delta.Text
			}
		case ev.Type == "content_block_delta" && ev.Delta.Type == "thinking_delta":
			if l := last(msgThinking); l != nil {
				l.text += ev.Delta.Thinking
			}
		}
	case "assistant":
		for _, b := range blocks(msg.Message) {
			if b.Type == "tool_use" {
				m.log = append(m.log, message{kind: msgTool, text: toolSummary(b.Name, b.Input)})
			}
		}
	case "user":
		for _, b := range blocks(msg.Message) {
			// Denials already have their own note.
			if line := firstLine(b.Content); b.Type == "tool_result" && b.IsError && !fromUser(line) {
				m.noteErr(line)
			}
		}
	case "control_request":
		var req struct {
			Subtype  string         `json:"subtype"`
			ToolName string         `json:"tool_name"`
			Input    map[string]any `json:"input"`
		}
		if json.Unmarshal(msg.Request, &req) != nil || req.Subtype != "can_use_tool" {
			return
		}
		m.perm, m.choice = &permission{id: msg.RequestID, tool: req.ToolName, input: req.Input}, 0
		m.omni.Blur() // keys answer the question until Tab opens a note
		// The tool call line turns into the question, and later into its answer.
		summary := toolSummary(req.ToolName, req.Input)
		m.perm.logIdx = -1
		for i := len(m.log) - 1; i >= 0 && m.perm.logIdx < 0; i-- {
			if m.log[i].kind == msgTool && m.log[i].text == summary {
				m.log[i].kind, m.perm.logIdx = msgAsk, i
			}
		}
		if m.perm.logIdx < 0 {
			m.perm.logIdx = len(m.log)
			m.log = append(m.log, message{kind: msgAsk, text: summary})
		}
		if reviewTools[req.ToolName] {
			m.openReview(req.ToolName, req.Input)
		}
	case "result":
		m.busy = false
		if m.interrupted {
			m.interrupted = false // the "interrupted" note is already there
			return
		}
		if msg.IsError {
			m.noteErr(msg.Result)
		} else {
			m.note(fmt.Sprintf("done · %.1fs · %d turns", float64(msg.Duration)/1000, msg.NumTurns))
		}
	}
}

// answerPermission resolves the pending tool request. note, if any, goes
// with the answer: as the reason of a rejection, in the message about the
// user's version, or as the next prompt after an approval. A proposal the
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
	if err := m.agent.Answer(p.id, allow, p.input, reason); err != nil {
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
			// An approval can't carry text; the note follows as the next prompt.
			m.queue = append(m.queue, note)
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
		m.note("  ⎿ queued: sent when Claude finishes")
		return nil
	}
	cmd := m.connect()
	if m.agent == nil {
		return nil
	}
	if err := m.agent.Prompt(text); err != nil {
		m.note(err.Error())
		return cmd
	}
	m.busy = true
	return cmd
}

// connect starts Claude Code if it is not running and returns the command
// that listens to it.
func (m *model) connect() tea.Cmd {
	resume := ""
	if m.agent != nil && m.agentStale && !m.busy {
		// New model or effort: restart Claude Code on the same session.
		resume = m.sessionID
		m.stopAgent()
	}
	if m.agent != nil {
		return nil
	}
	if m.resumeNext {
		resume, m.resumeNext = m.sessionID, false
	}
	dir, _ := os.Getwd()
	a, err := startAgent(dir, m.settings, resume)
	if err != nil {
		m.note(err.Error())
		return nil
	}
	m.agent = a
	return a.wait()
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

// agentSettingsChanged makes the next prompt restart Claude Code with the
// new model or effort, keeping the conversation.
func (m *model) agentSettingsChanged() {
	if m.agent != nil {
		m.agentStale = true
	}
	m.flash(modelName(m.settings.Model) + " · " + m.settings.Effort + " from the next message")
}

// stopAgent ends the agent session, if any.
func (m *model) stopAgent() {
	if m.agent != nil {
		m.agent.Close()
	}
	m.agent, m.busy, m.perm, m.agentReady, m.agentStale = nil, false, nil, false, false
	m.queue = nil
}

// choice is one answer to a pending tool call.
type choice struct{ key, label string }

// choices are the answers for the pending tool call: edits under review can
// also be edited before accepting.
func (m *model) choices() []choice {
	if m.mode == modeReview {
		return []choice{{"y", "Yes"}, {"e", "Edit first"}, {"n", "No"}}
	}
	return []choice{{"y", "Yes"}, {"n", "No"}}
}

// showChoices reports whether the choice list replaces the status line.
func (m *model) showChoices() bool {
	return m.perm != nil && (!m.fileMode() || !m.doc.Ed.On)
}

func (m *model) choose(key, note string) {
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
	out := []string{bg.Render(" ") + ask.Render("? Allow "+toolSummary(m.perm.tool, m.perm.input)+"?")}
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
