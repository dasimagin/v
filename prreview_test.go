package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"
)

const testDiff = "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,3 @@\n keep\n-old\n+new\n tail\n"

func testFlow(auto bool) *reviewFlow {
	return &reviewFlow{
		repo: "o/r", number: 7, auto: auto, out: make(chan tea.Msg, 16),
		data: &prData{Number: 7, Title: "T", Author: "a", State: "OPEN", Base: "main", Head: "feat", Files: splitDiff(testDiff)},
	}
}

func callTool(t *testing.T, rv *reviewFlow, name, args string) (string, error) {
	t.Helper()
	for _, tool := range prTools(rv) {
		if tool.Name == name {
			return tool.Handler(context.Background(), json.RawMessage(args))
		}
	}
	t.Fatalf("no tool %s", name)
	return "", nil
}

func TestPRCommentAutoAndRejections(t *testing.T) {
	rv := testFlow(true)
	if _, err := callTool(t, rv, "pr_comment", `{"path":"f.go","line":99,"level":"critical","body":"x"}`); err == nil || !strings.Contains(err.Error(), "not in the diff") {
		t.Errorf("a line outside the diff must be invalid: %v", err)
	}
	out, err := callTool(t, rv, "pr_comment", `{"path":"f.go","line":2,"level":"critical","body":"bug here"}`)
	if err != nil || !strings.HasPrefix(out, "accepted") {
		t.Fatalf("auto comment: %q %v", out, err)
	}
	if again, _ := callTool(t, rv, "pr_comment", `{"path":"f.go","line":2,"level":"critical","body":"bug here"}`); !strings.HasPrefix(again, "duplicate") {
		t.Errorf("same comment twice: %q", again)
	}
	if len(rv.cards) != 1 || rv.cards[0].Body != "**!!** bug here" || rv.cards[0].View.empty() {
		t.Errorf("cards = %+v", rv.cards)
	}
	payload, n := reviewPayload(rv.cards)
	var got struct {
		Event    string
		Comments []struct {
			Path string
			Line int
			Side string
			Body string
		}
	}
	if err := json.Unmarshal(payload, &got); err != nil || n != 1 || got.Event != "" || got.Comments[0].Side != "RIGHT" || got.Comments[0].Line != 2 {
		t.Errorf("payload = %s %v", payload, err)
	}
}

func TestPRCommentManualWaitsForTheUser(t *testing.T) {
	rv := testFlow(false)
	results := make(chan string, 2)
	call := func(line int, body string) {
		go func() {
			out, _ := callTool(t, rv, "pr_comment", fmt.Sprintf(`{"path":"f.go","line":%d,"level":"recommended","body":%q}`, line, body))
			results <- out
		}()
	}
	call(2, "first")
	ask := (<-rv.out).(reviewAskMsg)
	select {
	case out := <-results:
		t.Fatalf("returned before the user answered: %q", out)
	case <-time.After(50 * time.Millisecond):
	}
	ask.reply <- reviewReply{key: "n", note: "too picky"}
	if out := <-results; !strings.HasPrefix(out, "rejected") || !strings.Contains(out, "Do not post it again") || !strings.Contains(out, "too picky") {
		t.Errorf("rejection = %q", out)
	}
	call(3, "second")
	ask = (<-rv.out).(reviewAskMsg)
	ask.reply <- reviewReply{key: "y"}
	if out := <-results; !strings.HasPrefix(out, "accepted") {
		t.Errorf("acceptance = %q", out)
	}
	threads, _ := rv.threads("rejected")
	if !strings.Contains(threads, "[rejected]") || !strings.Contains(threads, "first") || strings.Contains(threads, "second") {
		t.Errorf("rejected threads = %q", threads)
	}
	if pending, _ := rv.threads("pending"); !strings.Contains(pending, "[pending]") || !strings.Contains(pending, "second") {
		t.Errorf("pending threads = %q", pending)
	}
}

func TestPRCommentStopsWhenTheReviewIsDropped(t *testing.T) {
	rv := testFlow(false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() { done <- rv.propose(ctx, &reviewCard{Path: "f.go", Line: 2, Body: "x"}) }()
	<-rv.out
	cancel()
	if out := <-done; !strings.HasPrefix(out, "review_ended") {
		t.Errorf("out = %q", out)
	}
}

func TestMCPServer(t *testing.T) {
	srv, err := startMCP([]mcpTool{{
		Name: "echo", Description: "d", Schema: `{"type":"object"}`,
		Handler: func(_ context.Context, args json.RawMessage) (string, error) { return "got " + string(args), nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	var cfg struct {
		McpServers map[string]struct {
			URL     string
			Headers map[string]string
		}
	}
	if err := json.Unmarshal([]byte(srv.Config("v")), &cfg); err != nil {
		t.Fatal(err)
	}
	c := cfg.McpServers["v"]
	post := func(auth, body string) (int, string) {
		req, _ := http.NewRequest("POST", c.URL, strings.NewReader(body))
		req.Header.Set("Authorization", auth)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, _ := post("Bearer wrong", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); code != http.StatusForbidden {
		t.Errorf("a wrong token got %d", code)
	}
	auth := c.Headers["Authorization"]
	if _, body := post(auth, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`); !strings.Contains(body, "2025-03-26") || !strings.Contains(body, "tools") {
		t.Errorf("initialize = %s", body)
	}
	if code, _ := post(auth, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusAccepted {
		t.Errorf("a notification got %d", code)
	}
	if _, body := post(auth, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`); !strings.Contains(body, `"name":"echo"`) || !strings.Contains(body, "inputSchema") {
		t.Errorf("tools/list = %s", body)
	}
	if _, body := post(auth, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"a":1}}}`); !strings.Contains(body, `got {\"a\":1}`) || !strings.Contains(body, `"isError":false`) {
		t.Errorf("tools/call = %s", body)
	}
	if _, body := post(auth, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"missing"}}`); !strings.Contains(body, "unknown tool") {
		t.Errorf("unknown tool = %s", body)
	}
}

func TestFindingCardShowsCommentUnderItsLine(t *testing.T) {
	m := &model{w: 80, h: 30}
	m.log = []message{{
		kind: msgFinding, file: "f.go:2", text: "**!!** bug here",
		view: diffView{Lines: []string{" keep", "+new", " tail"}, Nums: []int{1, 2, 3}, From: 1, To: 1},
	}}
	var out []string
	for _, l := range m.renderLog(20) {
		out = append(out, ansi.Strip(l))
	}
	text := strings.Join(out, "\n")
	at := func(s string) int { return strings.Index(text, s) }
	if !(at("f.go:2") >= 0 && at("+new") > at("f.go:2") && at("bug here") > at("+new") && at("tail") > at("bug here")) {
		t.Errorf("card layout:\n%s", text)
	}
}

func TestNotesReachTheReviewerWithTheNextToolResult(t *testing.T) {
	rv := testFlow(true)
	rv.addNote("focus on error handling")
	var list mcpTool
	for _, tool := range rv.withNotes(prTools(rv)) {
		if tool.Name == "pr_description" {
			list = tool
		}
	}
	out, err := list.Handler(context.Background(), json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out, "Note from the user: focus on error handling") {
		t.Fatalf("out = %q, %v", out, err)
	}
	if again, _ := list.Handler(context.Background(), json.RawMessage(`{}`)); strings.Contains(again, "Note from the user") {
		t.Error("a note must be shown once")
	}
}

func TestReviewQuestionUsesTheChoiceMenu(t *testing.T) {
	m := newModel()
	m.w, m.h, m.mode = 100, 30, modeChat
	rv := testFlow(false)
	rv.running = true
	m.rv = rv
	reply := make(chan reviewReply, 1)
	m.onReviewAsk(reviewAskMsg{rv, &reviewCard{Path: "f.go", Line: 2, Body: "x", label: "f.go:2"}, reply})
	if !m.showChoices() || len(m.choices()) != 4 {
		t.Fatalf("choices not shown: %v", m.choices())
	}
	if menu := ansi.Strip(strings.Join(m.renderChoices(), "\n")); !strings.Contains(menu, "Post this comment") || !strings.Contains(menu, "Stop the review") {
		t.Errorf("menu = %q", menu)
	}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	select {
	case r := <-reply:
		if r.key != "n" {
			t.Errorf("reply = %+v", r)
		}
	default:
		t.Fatal("the answer was not delivered")
	}
	if m.showChoices() {
		t.Error("the menu must close after the answer")
	}
}

func TestCheckedOutAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "one")
	first := run("rev-parse", "HEAD")
	d := &prData{Number: 7, Head: "feat", HeadSHA: first}
	if err := checkedOutAt(dir, d); err != nil {
		t.Errorf("at the head: %v", err)
	}
	run("commit", "--allow-empty", "-m", "two") // local commits on top are fine
	if err := checkedOutAt(dir, d); err != nil {
		t.Errorf("ahead of the head: %v", err)
	}
	d.HeadSHA = strings.Repeat("a", 40) // a commit this checkout does not have
	if err := checkedOutAt(dir, d); err == nil || !strings.Contains(err.Error(), "check out feat") {
		t.Errorf("a different head must be refused: %v", err)
	}
}

func TestCardsShowNumberedDiffWithTheCoveredLines(t *testing.T) {
	d := &prData{Files: splitDiff(rangeDiff)}
	d.Files[0].Threads = []prThread{{ID: "T1", Start: 2, Line: 3, Comments: []prComment{{Author: "alice", Body: "extract this"}}}}
	rv := &reviewFlow{data: d}
	v := d.Files[0].viewRange(2, 4)
	view, quote := rv.threadContext("T1")
	m := &model{w: 90, h: 40}
	m.log = []message{
		{kind: msgFinding, file: "f.go:2–4", text: "**!** helper", view: v},
		{kind: msgFinding, file: "reply", text: "done", view: view, quote: quote},
	}
	var out []string
	for _, l := range m.renderLog(40) {
		out = append(out, strings.TrimRight(ansi.Strip(l), " "))
	}
	text := strings.Join(out, "\n")
	for _, want := range []string{"   2 ▎  b", "   3 ▎ +c", "   4 ▎  d", "   1    a", "   5    e", "┆ @alice: extract this", "╭─ Claude · reply"} {
		if !strings.Contains(text, want) {
			t.Errorf("card lacks %q:\n%s", want, text)
		}
	}
	// The comment sits under the last covered line, before the context that follows.
	if !(strings.Index(text, "4 ▎  d") < strings.Index(text, "! helper") && strings.Index(text, "! helper") < strings.Index(text, "   5    e")) {
		t.Errorf("comment misplaced:\n%s", text)
	}
	if v := d.Files[0].viewRange(99, 99); !v.empty() {
		t.Error("a line outside the diff has no view")
	}
}

func TestReopenNeedsAResolvedThreadAndARealObjection(t *testing.T) {
	d := &prData{Number: 7, Files: []prFile{{Path: "f.go", Patch: strings.Split(strings.TrimRight(rangeDiff, "\n"), "\n"), Threads: []prThread{
		{ID: "T_closed", Line: 2, Resolved: true, Comments: []prComment{{Author: "me", Body: "fix this"}, {Author: "fixer", Body: "done"}}},
		{ID: "T_open", Line: 3, Comments: []prComment{{Author: "me", Body: "and this"}}},
	}}}}
	rv := &reviewFlow{data: d, auto: true, out: make(chan tea.Msg, 8)}
	call := func(args string) (string, error) { return callTool(t, rv, "pr_reopen", args) }

	for args, want := range map[string]string{
		`{"thread":"T_closed","level":"note","body":"x"}`:      "critical or recommended",
		`{"thread":"T_closed","level":"critical","body":"  "}`: "say why",
		`{"thread":"nope","level":"critical","body":"x"}`:      "no thread",
		`{"thread":"T_open","level":"recommended","body":"x"}`: "already open",
	} {
		if _, err := call(args); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", args, err, want)
		}
	}
	out, err := call(`{"thread":"T_closed","level":"critical","body":"the fix only moves the bug"}`)
	if err != nil || !strings.HasPrefix(out, "accepted") {
		t.Fatalf("reopen: %q %v", out, err)
	}
	c := rv.cards[0]
	if !c.Reopen || c.Thread != "T_closed" || c.Level != "critical" || c.Body != "**!!** the fix only moves the bug" || !strings.HasPrefix(c.label, "reopen") {
		t.Errorf("card = %+v", c)
	}
	if c.View.empty() || len(c.Quote) != 2 {
		t.Errorf("the card must show the thread: view=%v quote=%v", c.View, c.Quote)
	}
}

func TestLevelCountsTellBlockingFromNotes(t *testing.T) {
	rv := &reviewFlow{cards: []*reviewCard{
		{Level: "critical", Decision: "y"},
		{Level: "recommended", Decision: "y"},
		{Level: "recommended", Decision: "n"}, // turned down: not counted
		{Level: "note", Decision: "y"},
		{Level: "", Thread: "T", Decision: "y"}, // a plain reply
		{Level: "critical", Reopen: true, Thread: "T2", Decision: "y"},
	}}
	if blocking, notes := rv.levelCounts(); blocking != 3 || notes != 1 {
		t.Errorf("blocking=%d notes=%d", blocking, notes)
	}
	if blocking, notes := (&reviewFlow{}).levelCounts(); blocking != 0 || notes != 0 {
		t.Error("nothing proposed, nothing counted")
	}
}

func TestIterationsDefaultToFiveAndAreAnOption(t *testing.T) {
	s := defaultSettings()
	if s.Iterations != 5 {
		t.Errorf("default iterations = %d", s.Iterations)
	}
	m := newModel()
	m.settings = s
	var opt option
	for _, o := range options {
		if strings.HasPrefix(o.label, "Converge iterations") {
			opt = o
		}
	}
	if opt.label == "" {
		t.Fatal("no Converge iterations option")
	}
	if names := opt.values(&m); names[opt.get(&m)] != "5" {
		t.Errorf("the dropdown must start on 5: %v at %d", names, opt.get(&m))
	}
	opt.set(&m, 2)
	if m.settings.Iterations != 3 {
		t.Errorf("iterations = %d", m.settings.Iterations)
	}
}
