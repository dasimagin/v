package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAutoDenyOnlyForcePushes(t *testing.T) {
	deny := func(cmd string) bool { return autoDeny("Bash", map[string]any{"command": cmd}) != "" }
	for _, cmd := range []string{"git push --force", "git push -f origin feat", "git push origin +feat", "git push --force-with-lease", "git push origin feat --force"} {
		if !deny(cmd) {
			t.Errorf("%q must be denied", cmd)
		}
	}
	for _, cmd := range []string{"git push", "git push origin feat", "git commit -m 'fix --force handling'", "go test ./...", "bazel build //..."} {
		if deny(cmd) {
			t.Errorf("%q must be allowed", cmd)
		}
	}
	if autoDeny("Edit", map[string]any{"file_path": "x"}) != "" {
		t.Error("edits are not restricted")
	}
}

// fakeAgent records the answers to permission requests.
type fakeAgent struct {
	backend
	allow  []bool
	reason []string
}

func (f *fakeAgent) Answer(_ *permission, allow bool, reason string) error {
	f.allow, f.reason = append(f.allow, allow), append(f.reason, reason)
	return nil
}

func TestApplyPermissionByMode(t *testing.T) {
	for _, c := range []struct {
		name    string
		auto    bool
		tool    string
		input   map[string]any
		handled bool
		allowed bool
	}{
		{"own tools need no prompt, manual", false, "mcp__v__pr_reply", nil, true, true},
		{"own tools need no prompt, auto", true, "mcp__v__pr_resolve", nil, true, true},
		{"manual asks the user about edits", false, "Edit", map[string]any{"file_path": "a"}, false, false},
		{"manual asks the user about the shell", false, "Bash", map[string]any{"command": "git push"}, false, false},
		{"auto allows edits", true, "Edit", map[string]any{"file_path": "a"}, true, true},
		{"auto allows git and builds", true, "Bash", map[string]any{"command": "git commit -am x"}, true, true},
		{"auto refuses a force push", true, "Bash", map[string]any{"command": "git push --force"}, true, false},
	} {
		m := newModel()
		fa := &fakeAgent{}
		m.agent = fa
		m.rv = &reviewFlow{apply: true, auto: c.auto}
		m.runScr = &runScreen{kind: "apply", rv: m.rv}
		handled := m.runPermission(&permission{tool: c.tool, input: c.input})
		if handled != c.handled {
			t.Errorf("%s: handled = %v", c.name, handled)
			continue
		}
		if handled && (len(fa.allow) != 1 || fa.allow[0] != c.allowed) {
			t.Errorf("%s: answers = %v", c.name, fa.allow)
		}
	}
	// Outside an apply run nothing is decided for the user.
	m := newModel()
	m.agent = &fakeAgent{}
	if m.runPermission(&permission{tool: "mcp__v__pr_reply"}) {
		t.Error("no run: leave it to the usual prompt")
	}
}

func TestApplyToolsAskInManualAndActInAuto(t *testing.T) {
	hub := &applyHub{}
	call := func(name, args string) (string, error) {
		for _, tool := range applyTools(hub) {
			if tool.Name == name {
				return tool.Handler(context.Background(), json.RawMessage(args))
			}
		}
		t.Fatalf("no tool %s", name)
		return "", nil
	}
	names := map[string]bool{}
	for _, tool := range applyTools(hub) {
		names[tool.Name] = true
	}
	for _, want := range []string{"pr_description", "pr_threads", "pr_checks", "pr_reply", "pr_resolve"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
	for _, gone := range []string{"pr_list", "pr_diff", "pr_file", "pr_comment"} {
		if names[gone] {
			t.Errorf("tool %s must not be offered while applying", gone)
		}
	}
	if _, err := call("pr_reply", `{"thread":"T1","body":"done"}`); err == nil || !strings.Contains(err.Error(), "/apply") {
		t.Errorf("no run: %v", err)
	}

	// Manual: the call waits for the user, and a refusal is reported with its note.
	rv := &reviewFlow{apply: true, repo: "o/r", out: make(chan tea.Msg, 4), data: &prData{Number: 7}}
	hub.set(rv)
	result := make(chan string, 1)
	go func() {
		out, _ := call("pr_resolve", `{"thread":"T1"}`)
		result <- out
	}()
	ask := (<-rv.out).(reviewAskMsg)
	if ask.card.Question != "Resolve this thread?" || ask.card.Thread != "T1" {
		t.Errorf("card = %+v", ask.card)
	}
	select {
	case out := <-result:
		t.Fatalf("returned before the answer: %q", out)
	case <-time.After(50 * time.Millisecond):
	}
	ask.reply <- reviewReply{key: "n", note: "not yet fixed"}
	if out := <-result; !strings.HasPrefix(out, "declined") || !strings.Contains(out, "not yet fixed") || !strings.Contains(out, "Do not retry") {
		t.Errorf("declined = %q", out)
	}

	// A stopped run ends the call.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() {
		out, _ := applyTools(hub)[len(applyTools(hub))-1].Handler(ctx, json.RawMessage(`{"thread":"T1"}`))
		done <- out
	}()
	<-rv.out
	cancel()
	if out := <-done; !strings.HasPrefix(out, "run_ended") {
		t.Errorf("stopped = %q", out)
	}

	// Auto: nothing is asked; the card is only shown, and the tool goes on to act
	// (here gh is unavailable or the thread is fake, so it fails after the gate).
	rv.auto = true
	go func() { call("pr_reply", `{"thread":"T9","body":"fixed in abc"}`) }()
	if shown, ok := (<-rv.out).(reviewShownMsg); !ok || shown.card.Thread != "T9" {
		t.Errorf("auto must show, not ask: %+v", shown)
	}
}

func TestFinishRunTurnReleasesTheTurnAndKeepsTheRun(t *testing.T) {
	m := newModel()
	rv := &reviewFlow{apply: true, out: make(chan tea.Msg, 4)}
	m.rv = rv
	m.runScr = &runScreen{kind: "apply", rv: rv}
	m.hub.set(rv)
	m.finishRunTurn()
	if m.rv != nil {
		t.Error("the turn must be over")
	}
	if m.hub.get() == nil || m.runScr == nil {
		t.Error("the run and its tools stay until it is left")
	}
	if _, ok := (<-rv.out).(reviewEndMsg); !ok {
		t.Error("the waiting command must be released")
	}
	m.finishRunTurn() // idempotent
}

func TestReviewTurnPostsWhatWasAcceptedOnce(t *testing.T) {
	m := newModel()
	rv := &reviewFlow{repo: "o/r", number: 7, out: make(chan tea.Msg, 4), cards: []*reviewCard{
		{Path: "f.go", Line: 2, Body: "a", Decision: "y"},
		{Path: "f.go", Line: 3, Body: "b", Decision: "n"},
	}}
	m.rv, m.runScr = rv, &runScreen{kind: "review", rv: rv}
	m.finishRunTurn()
	if m.deferred == nil || !rv.cards[0].posted || rv.cards[1].posted {
		t.Fatalf("accepted card must be on its way, rejected untouched: deferred=%v posted=%v/%v", m.deferred != nil, rv.cards[0].posted, rv.cards[1].posted)
	}
	// A second turn with nothing new accepted posts nothing.
	m.deferred = nil
	m.rv = rv
	m.finishRunTurn()
	if m.deferred != nil {
		t.Error("nothing new was accepted")
	}
	// A dropped review sends nothing and forgets the accepted comments.
	rv2 := &reviewFlow{out: make(chan tea.Msg, 4), dropped: true, cards: []*reviewCard{{Decision: "y", Body: "x"}}}
	m.rv, m.runScr = rv2, &runScreen{kind: "review", rv: rv2}
	m.finishRunTurn()
	if m.deferred != nil || !rv2.cards[0].posted {
		t.Error("a dropped review must not be posted")
	}
}

func TestReviewerMayOnlyRead(t *testing.T) {
	for cmd, ok := range map[string]bool{
		"git diff origin/main...HEAD": true, "git log --oneline -5": true, "git show abc123": true, "git blame f.go": true,
		"git push": false, "git commit -am x": false, "git diff; rm -rf /": false, "git log | sh": false, "go test ./...": false, "git diff > out": false,
	} {
		if got := readOnlyGit(cmd); got != ok {
			t.Errorf("readOnlyGit(%q) = %v", cmd, got)
		}
	}
	m := newModel()
	fa := &fakeAgent{}
	m.agent = fa
	rv := &reviewFlow{} // a review: apply is false
	m.rv, m.runScr = rv, &runScreen{kind: "review", rv: rv}
	for _, c := range []struct {
		tool  string
		input map[string]any
		allow bool
	}{
		{"Read", nil, true},
		{"Grep", nil, true},
		{"mcp__v__pr_comment", nil, true},
		{"Bash", map[string]any{"command": "git diff HEAD~1"}, true},
		{"Bash", map[string]any{"command": "go test ./..."}, false},
		{"Edit", map[string]any{"file_path": "f.go"}, false},
	} {
		fa.allow = nil
		if !m.runPermission(&permission{tool: c.tool, input: c.input}) || len(fa.allow) != 1 || fa.allow[0] != c.allow {
			t.Errorf("%s %v: answers = %v", c.tool, c.input, fa.allow)
		}
	}
}

func TestRunScreenKeepsTheChatAside(t *testing.T) {
	m := newModel()
	m.w, m.h, m.mode = 100, 30, modeChat
	m.hub = &applyHub{}
	m.log = []message{{kind: msgUser, text: "my own conversation"}}
	m.sessionID = "main-session"
	m.queue = []string{"later"}
	if !m.enterRun("apply", "PR #7", true) {
		t.Fatal("could not enter the run")
	}
	if len(m.log) != 0 || m.sessionID != "" || m.queue != nil || m.agent != nil {
		t.Errorf("the run must start empty: log=%d session=%q", len(m.log), m.sessionID)
	}
	m.note("the run says hello")
	if m.enterRun("review", "PR #8", false) {
		t.Error("a second run must be refused while one is open")
	}
	if title := ansi.Strip(strings.Join(m.chatHeader(), "\n")); !strings.Contains(title, "Apply · PR #7") || !strings.Contains(title, "esc to leave") {
		t.Errorf("header = %q", title)
	}
	// Esc on an idle run leaves it.
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyEsc}) // key has a value receiver: take the result
	m = next.(model)
	if m.runScr != nil {
		t.Fatal("esc must leave a finished run")
	}
	if len(m.log) < 1 || m.log[0].text != "my own conversation" || m.sessionID != "main-session" || !m.resumeNext || len(m.queue) != 1 {
		t.Errorf("the chat must be back: log=%v session=%q resume=%v", m.log, m.sessionID, m.resumeNext)
	}
	if !strings.Contains(m.workspaceNotice, "/apply") {
		t.Errorf("the chat agent must be told about the run: %q", m.workspaceNotice)
	}
	if m.toolsOn {
		t.Error("the chat agent must not keep the run's tools")
	}
}

func TestRunScreenRoutesTypingAndBlocksCommands(t *testing.T) {
	m := newModel()
	m.w, m.h, m.mode = 100, 30, modeChat
	m.hub = &applyHub{}
	m.enterRun("apply", "PR #7", false)
	rv := &reviewFlow{apply: true, running: true, out: make(chan tea.Msg, 4)}
	m.rv, m.runScr.rv = rv, rv
	m.busy = true
	// While the agent works, typed text is a note delivered with its next tool result.
	if cmd := m.run("keep the diff small"); cmd != nil {
		t.Error("a note is not a prompt")
	}
	if got := rv.takeNotes(); got != "keep the diff small" {
		t.Errorf("note = %q", got)
	}
	// Commands that would change the ground under the run are refused.
	m.setErr("")
	m.run("/reset")
	if !strings.Contains(m.status, "leave the apply run") {
		t.Errorf("status = %q", m.status)
	}
	// Esc while the agent works stops it (and keeps the screen), a later Esc leaves.
	stopped := false
	rv.cancel = func() { stopped = true }
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.runScr == nil || !rv.dropped || !stopped {
		t.Error("esc must stop the agent, not leave")
	}
}
