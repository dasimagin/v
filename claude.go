package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// claudeAgent is a long-running `claude -p` process speaking stream-json on
// stdin and stdout. It uses the user's Claude Code login: ANTHROPIC_API_KEY
// is removed from its environment so the subscription is used, not an API key.
type claudeAgent struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	mu     sync.Mutex
	events chan ev
	stderr bytes.Buffer
	reqN   int
	mcp    *mcpServer // v's tools for this session, if any
}

// claudeMsg is one line from the CLI; only the fields v uses are decoded.
type claudeMsg struct {
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

// startClaude runs Claude Code in dir with the model and effort from s;
// resume, if set, continues that session.
func startClaude(dir string, s Settings, resume string, tools []mcpTool, system string) (*claudeAgent, error) {
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
		"--append-system-prompt", strings.TrimSpace(systemPrompt+"\n\n"+system),
		"--model", model, "--effort", s.Effort,
		"--thinking-display", "summarized", // without it thinking blocks come empty
	)
	if resume != "" {
		cmd.Args = append(cmd.Args, "--resume", resume)
	}
	var srv *mcpServer
	if len(tools) > 0 {
		var err error
		if srv, err = startMCP(tools); err != nil {
			return nil, err
		}
		cmd.Args = append(cmd.Args, "--mcp-config", srv.Config("v")) // alongside the user's own MCP servers
	}
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	a := &claudeAgent{cmd: cmd, events: make(chan ev, 256), mcp: srv}
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
		if srv != nil {
			srv.Close()
		}
		return nil, fmt.Errorf("start claude: %w", err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var m claudeMsg
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				for _, e := range claudeEvents(m, dir) {
					a.events <- e
				}
			}
		}
		_ = cmd.Wait()
		close(a.events)
	}()
	return a, nil
}

func (a *claudeAgent) Name() string { return "Claude Code" }

func (a *claudeAgent) wait() tea.Cmd {
	return waitFor(a, a.events, func() string { return strings.TrimSpace(a.stderr.String()) })
}

func (a *claudeAgent) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err = a.in.Write(append(b, '\n'))
	return err
}

func (a *claudeAgent) Prompt(text string) error {
	return a.send(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": text},
		"parent_tool_use_id": nil,
		"session_id":         "default",
	})
}

func (a *claudeAgent) Interrupt() error {
	a.mu.Lock()
	a.reqN++
	id := fmt.Sprintf("v_%d", a.reqN)
	a.mu.Unlock()
	return a.send(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "interrupt"}})
}

// Answer replies to a can_use_tool request.
func (a *claudeAgent) Answer(p *permission, allow bool, reason string) error {
	body := map[string]any{"behavior": "deny", "message": reason}
	if allow {
		body = map[string]any{"behavior": "allow", "updatedInput": p.input}
	}
	return a.send(map[string]any{
		"type":     "control_response",
		"response": map[string]any{"subtype": "success", "request_id": p.id, "response": body},
	})
}

func (a *claudeAgent) Steer(string) error { return errNoSteer }

// Close ends the session; the process exits when its stdin closes.
func (a *claudeAgent) Close() {
	_ = a.in.Close()
	if a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
	if a.mcp != nil {
		a.mcp.Close()
	}
}

// claudeEvents translates one CLI message.
func claudeEvents(msg claudeMsg, dirs ...string) []ev {
	switch msg.Type {
	case "system":
		if msg.Subtype == "init" {
			return []ev{{kind: evReady, model: msg.Model, session: msg.SessionID}}
		}
	case "stream_event":
		var se struct {
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
		if json.Unmarshal(msg.Event, &se) != nil {
			return nil
		}
		switch {
		case se.Type == "content_block_start" && se.ContentBlock.Type == "text":
			return []ev{{kind: evTextStart}}
		case se.Type == "content_block_start" && se.ContentBlock.Type == "thinking":
			return []ev{{kind: evThinkingStart}}
		case se.Type == "content_block_delta" && se.Delta.Type == "text_delta":
			return []ev{{kind: evText, text: se.Delta.Text}}
		case se.Type == "content_block_delta" && se.Delta.Type == "thinking_delta":
			return []ev{{kind: evThinking, text: se.Delta.Thinking}}
		}
	case "assistant":
		var out []ev
		for _, b := range blocks(msg.Message) {
			if b.Type == "tool_use" {
				out = append(out, ev{kind: evTool, text: toolSummary(b.Name, b.Input, dirs...)})
			}
		}
		return out
	case "user":
		var out []ev
		for _, b := range blocks(msg.Message) {
			// Denials already show on the question line.
			if line := firstLine(b.Content); b.Type == "tool_result" && b.IsError && !fromUser(line) {
				out = append(out, ev{kind: evToolError, text: line})
			}
		}
		return out
	case "control_request":
		var req struct {
			Subtype  string         `json:"subtype"`
			ToolName string         `json:"tool_name"`
			Input    map[string]any `json:"input"`
		}
		if json.Unmarshal(msg.Request, &req) != nil || req.Subtype != "can_use_tool" {
			return nil
		}
		p := &permission{id: msg.RequestID, summary: toolSummary(req.ToolName, req.Input, dirs...), tool: req.ToolName, input: req.Input}
		if reviewTools[req.ToolName] {
			p.propose = func() (*Doc, error) { return proposedDoc(req.ToolName, req.Input, dirs...) }
		}
		return []ev{{kind: evAsk, perm: p}}
	case "result":
		return []ev{{kind: evDone, isErr: msg.IsError, text: msg.Result, secs: float64(msg.Duration) / 1000, turns: msg.NumTurns}}
	}
	return nil
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
