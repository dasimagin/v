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
)

// The agent always runs on this model; there is no setting for it yet.
const (
	agentModel  = "claude-opus-5-5"
	agentEffort = "high"
)

// systemPrompt is appended to Claude Code's own system prompt.
const systemPrompt = `You are running inside v, a terminal UI for coding together with the user.
The user sees every edit you propose as a diff and approves or rejects it. Keep replies short.`

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

func startAgent(dir string) (*Agent, error) {
	model := agentModel
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
		"--model", model, "--effort", agentEffort,
		"--thinking-display", "summarized", // without it thinking blocks come empty
	)
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
	id    string
	tool  string
	input map[string]any
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
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
	switch msg.Type {
	case "system":
		if msg.Subtype == "init" && !m.agentReady {
			m.agentReady = true
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
			if line := firstLine(b.Content); b.Type == "tool_result" && b.IsError && line != denyMessage {
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
		m.perm = &permission{id: msg.RequestID, tool: req.ToolName, input: req.Input}
		m.log = append(m.log, message{kind: msgAsk, text: "Allow " + toolSummary(req.ToolName, req.Input) + "?  y / n"})
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

// answerPermission resolves the pending tool request.
func (m *model) answerPermission(allow bool) {
	p := m.perm
	m.perm = nil
	if err := m.agent.Answer(p.id, allow, p.input, denyMessage); err != nil {
		m.setErr(err.Error())
	}
	if allow {
		m.note("  ⎿ allowed")
	} else {
		m.note("  ⎿ denied")
	}
	if m.mode == modeReview {
		m.pop()
	}
}

// prompt sends text to the agent, starting it on first use.
func (m *model) prompt(text string) tea.Cmd {
	if m.busy {
		m.note("the agent is busy · esc to interrupt")
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
	if m.agent != nil {
		return nil
	}
	dir, _ := os.Getwd()
	a, err := startAgent(dir)
	if err != nil {
		m.note(err.Error())
		return nil
	}
	m.agent = a
	return a.wait()
}

// stopAgent ends the agent session, if any.
func (m *model) stopAgent() {
	if m.agent != nil {
		m.agent.Close()
	}
	m.agent, m.busy, m.perm, m.agentReady = nil, false, nil, false
}
