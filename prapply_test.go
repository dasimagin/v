package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
		handled := m.applyPermission(&permission{tool: c.tool, input: c.input})
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
	if m.applyPermission(&permission{tool: "mcp__v__pr_reply"}) {
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

func TestFinishApplyReleasesTheRun(t *testing.T) {
	m := newModel()
	rv := &reviewFlow{apply: true, out: make(chan tea.Msg, 4)}
	m.rv = rv
	m.hub.set(rv)
	m.finishApply()
	if m.rv != nil || m.hub.get() != nil {
		t.Error("the run must be cleared")
	}
	if _, ok := (<-rv.out).(reviewEndMsg); !ok {
		t.Error("the waiting command must be released")
	}
	m.finishApply() // idempotent
	// A review (not an apply) is left alone.
	m.rv = &reviewFlow{apply: false}
	m.finishApply()
	if m.rv == nil {
		t.Error("finishApply must not end a /review")
	}
}

func TestReplyEndpointHasThePullNumber(t *testing.T) {
	if got := replyEndpoint("o/r", 7, 99); got != "repos/o/r/pulls/7/comments/99/replies" {
		t.Errorf("endpoint = %q", got)
	}
}

func TestThreadLabelNamesThePlace(t *testing.T) {
	rv := &reviewFlow{data: &prData{Files: []prFile{{Path: "git.go", Threads: []prThread{{ID: "T1", Line: 42, Comments: []prComment{{Author: "alice"}}}}}}}}
	if got := rv.threadLabel("T1"); got != "git.go:42 (@alice)" {
		t.Errorf("label = %q", got)
	}
	if got := rv.threadLabel("nope"); got != "thread nope" {
		t.Errorf("unknown = %q", got)
	}
}
