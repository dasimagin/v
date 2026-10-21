package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const agentCodex = "codex"

// codexAgent drives `codex app-server`: JSON-RPC over stdio, the protocol
// of Codex's IDE integrations. It uses the user's Codex login (ChatGPT).
// The thread runs in a read-only sandbox with approvals on request, so
// reads go through and every edit or writing command asks first.
type codexAgent struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	mu     sync.Mutex
	events chan ev
	stderr bytes.Buffer

	nextID  int
	pending map[int]string // request id -> method, to read responses

	// Touched only by the reader goroutine, except where noted.
	threadID string
	turnID   string // the running turn; read by Interrupt and Steer under mu
	changes  map[string][]codexChange
	model    string
	effort   string
	queued   []string // prompts sent before the thread was ready; under mu
	resume   string
	dir      string
	started  time.Time
}

type codexChange struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
	Kind struct {
		Type string `json:"type"` // add, delete, update
	} `json:"kind"`
}

// rpcMsg is any JSON-RPC message: a request or notification (method), or a
// response (result or error).
type rpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func startCodex(dir string, s Settings, resume string) (*codexAgent, error) {
	cmd := exec.Command("codex", "app-server")
	cmd.Dir = dir
	a := &codexAgent{cmd: cmd, events: make(chan ev, 256), pending: map[int]string{},
		changes: map[string][]codexChange{}, model: s.CodexModel, effort: s.CodexEffort, resume: resume, dir: dir}
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
		return nil, fmt.Errorf("start codex: %w", err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var m rpcMsg
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				a.handle(m)
			}
		}
		_ = cmd.Wait()
		close(a.events)
	}()
	_, err = a.request("initialize", map[string]any{"clientInfo": map[string]any{"name": "v", "version": "0.1"}})
	return a, err
}

func (a *codexAgent) Name() string { return "Codex" }

func (a *codexAgent) wait() tea.Cmd {
	return waitFor(a, a.events, func() string { return lastLines(a.stderr.String(), 3) })
}

// lastLines keeps the end of a log, which is where the error usually is.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(len(lines)-n, 0):], "\n")
}

func (a *codexAgent) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = a.in.Write(append(b, '\n'))
	return err
}

// request sends a JSON-RPC request; its response comes to handle.
func (a *codexAgent) request(method string, params any) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nextID++
	a.pending[a.nextID] = method
	return a.nextID, a.write(map[string]any{"jsonrpc": "2.0", "id": a.nextID, "method": method, "params": params})
}

func (a *codexAgent) notify(method string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.write(map[string]any{"jsonrpc": "2.0", "method": method})
}

// respond answers a request from the server.
func (a *codexAgent) respond(id any, result any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (a *codexAgent) respondError(id json.RawMessage, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = a.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32601, "message": msg}})
}

func (a *codexAgent) emit(e ev) { a.events <- e }

// handle reacts to one message from the server.
func (a *codexAgent) handle(m rpcMsg) {
	switch {
	case m.Method != "" && len(m.ID) > 0:
		a.serverRequest(m)
	case m.Method != "":
		a.notification(m)
	case len(m.ID) > 0:
		a.response(m)
	}
}

func (a *codexAgent) response(m rpcMsg) {
	var id int
	_ = json.Unmarshal(m.ID, &id)
	a.mu.Lock()
	method := a.pending[id]
	delete(a.pending, id)
	a.mu.Unlock()
	if m.Error != nil {
		a.emit(ev{kind: evDone, isErr: true, text: method + ": " + m.Error.Message})
		return
	}
	switch method {
	case "initialize":
		_ = a.notify("initialized")
		params := map[string]any{
			"cwd": a.dir, "approvalPolicy": "on-request", "sandbox": "read-only",
			"developerInstructions": chatPrompt(a.dir),
		}
		if a.resume != "" {
			params["threadId"] = a.resume
			_, _ = a.request("thread/resume", params)
		} else {
			_, _ = a.request("thread/start", params)
		}
	case "thread/start", "thread/resume":
		var r struct {
			Model  string `json:"model"`
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		_ = json.Unmarshal(m.Result, &r)
		a.mu.Lock()
		a.threadID = r.Thread.ID
		queued := a.queued
		a.queued = nil
		a.mu.Unlock()
		model := r.Model
		if a.model != "" {
			model = a.model
		}
		a.emit(ev{kind: evReady, model: codexModelName(model), session: r.Thread.ID})
		for _, text := range queued {
			_ = a.Prompt(text)
		}
	}
}

func (a *codexAgent) notification(m rpcMsg) {
	var p struct {
		Delta string `json:"delta"`
		Turn  struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			DurationMs int    `json:"durationMs"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
		Item struct {
			Type     string        `json:"type"`
			ID       string        `json:"id"`
			Command  string        `json:"command"`
			Changes  []codexChange `json:"changes"`
			Tool     string        `json:"tool"`
			Query    string        `json:"query"`
			ExitCode *int          `json:"exitCode"`
			Status   string        `json:"status"`
		} `json:"item"`
	}
	_ = json.Unmarshal(m.Params, &p)
	switch m.Method {
	case "turn/started":
		a.mu.Lock()
		a.turnID = p.Turn.ID
		a.mu.Unlock()
		a.started = time.Now()
	case "turn/completed":
		a.mu.Lock()
		a.turnID = ""
		a.mu.Unlock()
		e := ev{kind: evDone, secs: time.Since(a.started).Seconds()}
		if p.Turn.Status == "failed" && p.Turn.Error != nil {
			e.isErr, e.text = true, codexErrorText(p.Turn.Error.Message)
		}
		a.emit(e)
	case "item/started":
		switch p.Item.Type {
		case "agentMessage":
			a.emit(ev{kind: evTextStart})
		case "reasoning":
			a.emit(ev{kind: evThinkingStart})
		case "commandExecution":
			a.emit(ev{kind: evTool, text: "Bash " + shellCommand(p.Item.Command)})
		case "fileChange":
			a.changes[p.Item.ID] = p.Item.Changes
			a.emit(ev{kind: evTool, text: changeSummary(p.Item.Changes, a.dir)})
		case "mcpToolCall":
			a.emit(ev{kind: evTool, text: p.Item.Tool})
		case "webSearch":
			a.emit(ev{kind: evTool, text: "WebSearch " + p.Item.Query})
		}
	case "item/completed":
		if p.Item.Type == "commandExecution" && p.Item.ExitCode != nil && *p.Item.ExitCode != 0 && p.Item.Status != "declined" {
			a.emit(ev{kind: evToolError, text: fmt.Sprintf("exit code %d", *p.Item.ExitCode)})
		}
	case "item/agentMessage/delta":
		a.emit(ev{kind: evText, text: p.Delta})
	case "item/reasoning/summaryTextDelta":
		a.emit(ev{kind: evThinking, text: p.Delta})
	}
}

func (a *codexAgent) serverRequest(m rpcMsg) {
	var p struct {
		ItemID  string `json:"itemId"`
		Command string `json:"command"`
	}
	_ = json.Unmarshal(m.Params, &p)
	switch m.Method {
	case "item/commandExecution/requestApproval":
		a.emit(ev{kind: evAsk, perm: &permission{id: m.ID, summary: "Bash " + shellCommand(p.Command)}})
	case "item/fileChange/requestApproval":
		changes := a.changes[p.ItemID]
		perm := &permission{id: m.ID, summary: changeSummary(changes, a.dir)}
		if len(changes) == 1 {
			c := changes[0]
			perm.propose = func() (*Doc, error) { return codexProposal(c, a.dir) }
		}
		a.emit(ev{kind: evAsk, perm: perm})
	default:
		// Elicitations, extra permissions and the like: v doesn't do them yet.
		a.respondError(m.ID, "v does not support "+m.Method)
	}
}

func (a *codexAgent) Prompt(text string) error {
	a.mu.Lock()
	thread := a.threadID
	if thread == "" {
		a.queued = append(a.queued, text) // sent once the thread is ready
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	params := map[string]any{"threadId": thread, "input": []any{map[string]any{"type": "text", "text": text}}}
	if a.model != "" {
		params["model"] = a.model
	}
	if a.effort != "" {
		params["effort"] = a.effort
	}
	_, err := a.request("turn/start", params)
	return err
}

func (a *codexAgent) Interrupt() error {
	a.mu.Lock()
	thread, turn := a.threadID, a.turnID
	a.mu.Unlock()
	if turn == "" {
		return nil
	}
	_, err := a.request("turn/interrupt", map[string]any{"threadId": thread, "turnId": turn})
	return err
}

// Steer adds a user message to the running turn.
func (a *codexAgent) Steer(text string) error {
	a.mu.Lock()
	thread, turn := a.threadID, a.turnID
	a.mu.Unlock()
	if turn == "" {
		return errNoSteer
	}
	_, err := a.request("turn/steer", map[string]any{
		"threadId": thread, "expectedTurnId": turn,
		"input": []any{map[string]any{"type": "text", "text": text}},
	})
	return err
}

// Answer accepts or declines; a decline can't carry a reason, so the
// reason follows as a message in the same turn.
func (a *codexAgent) Answer(p *permission, allow bool, reason string) error {
	decision := "decline"
	if allow {
		decision = "accept"
	}
	if err := a.respond(p.id, map[string]any{"decision": decision}); err != nil {
		return err
	}
	if !allow && reason != denyMessage {
		if err := a.Steer(reason); err != nil && !errors.Is(err, errNoSteer) {
			return err
		}
	}
	return nil
}

func (a *codexAgent) Close() {
	_ = a.in.Close()
	if a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
}

// shellCommand drops the shell wrapper Codex runs commands in.
func shellCommand(c string) string {
	for _, sh := range []string{"/bin/zsh -lc ", "/bin/bash -lc ", "bash -lc ", "zsh -lc "} {
		if strings.HasPrefix(c, sh) {
			c = strings.TrimPrefix(c, sh)
			if len(c) >= 2 && (c[0] == '"' || c[0] == '\'') && c[len(c)-1] == c[0] {
				c = c[1 : len(c)-1]
			}
			break
		}
	}
	return c
}

func changeSummary(changes []codexChange, dirs ...string) string {
	switch len(changes) {
	case 0:
		return "Edit"
	case 1:
		verb := "Edit"
		switch changes[0].Kind.Type {
		case "add":
			verb = "Write"
		case "delete":
			verb = "Delete"
		}
		return verb + " " + shortPath(changes[0].Path, dirs...)
	}
	return fmt.Sprintf("Edit %d files", len(changes))
}

// codexProposal builds the file as Codex's change would leave it.
func codexProposal(c codexChange, dirs ...string) (*Doc, error) {
	if len(dirs) > 0 {
		c.Path = workPath(dirs[0], c.Path)
	}
	cur, err := os.ReadFile(c.Path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var next string
	switch c.Kind.Type {
	case "delete":
		next = ""
	case "add":
		next = c.Diff
		if strings.HasPrefix(c.Diff, "@@") {
			lines, err := applyHunks(nil, c.Diff)
			if err != nil {
				return nil, err
			}
			next = strings.Join(lines, "\n") + "\n"
		}
	default:
		old, err := newDoc(c.Path, string(cur))
		if err != nil {
			return nil, err
		}
		lines, err := applyHunks(old.Text, c.Diff)
		if err != nil {
			return nil, err
		}
		old.Text = lines
		next = old.Contents()
	}
	d, err := newDoc(c.Path, next)
	if err != nil {
		return nil, err
	}
	verb, _, _ := strings.Cut(changeSummary([]codexChange{c}), " ")
	d.Base, d.Review, d.Orig = &Base{New: true}, verb, append([]string(nil), d.Text...)
	if exists {
		old, err := newDoc(c.Path, string(cur))
		if err != nil {
			return nil, err
		}
		d.Base = &Base{Lines: old.Lines, Text: old.Text}
	}
	return d, nil
}

// codexErrorText pulls the message out of an error that may hold JSON.
func codexErrorText(s string) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(s), &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return s
}
