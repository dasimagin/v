package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestLoopNextGivesTheReviewerTheLastWord(t *testing.T) {
	for _, c := range []struct {
		name         string
		r            roundSummary
		open         int
		iter, max    int
		again        bool
		reasonSubstr string
	}{
		{"a blocking comment: look again", roundSummary{critical: 1}, 0, 1, 5, true, ""},
		{"a recommended one counts too", roundSummary{recommended: 2, notes: 1}, 0, 2, 5, true, ""},
		{"only notes, all closed: done", roundSummary{notes: 3}, 0, 1, 5, false, "only notes"},
		{"only notes, but the fixer left a thread open: look again", roundSummary{notes: 3}, 1, 1, 5, true, ""},
		{"blocking in the last round: the limit ends it", roundSummary{critical: 1}, 0, 5, 5, false, "5 rounds are used up"},
		{"open threads in the last round", roundSummary{notes: 1}, 2, 3, 3, false, "2 threads still open"},
		{"a blocking comment, everything closed, rounds left: still look again", roundSummary{recommended: 1}, 0, 1, 2, true, ""},
	} {
		again, reason := loopNext(&c.r, c.open, c.iter, c.max)
		if again != c.again || !strings.Contains(reason, c.reasonSubstr) {
			t.Errorf("%s: again=%v reason=%q", c.name, again, reason)
		}
	}
}

func TestReviewerCardsAreCountedAndTakenOnce(t *testing.T) {
	rv := &reviewFlow{cards: []*reviewCard{
		{Level: "critical", Decision: "y"},
		{Level: "recommended", Decision: "y"},
		{Level: "recommended", Decision: "n"},
		{Level: "note", Decision: "y"},
		{Level: "note", Decision: "y"},
		{Thread: "T", Decision: "y"}, // a reply
	}}
	if c, r, n := rv.levelBreakdown(); c != 1 || r != 1 || n != 2 {
		t.Errorf("breakdown = %d/%d/%d", c, r, n)
	}
	if got := rv.takeAccepted(); len(got) != 5 {
		t.Errorf("taken = %d, want the 5 accepted ones", len(got))
	}
	if got := rv.takeAccepted(); len(got) != 0 {
		t.Error("a proposal is taken once")
	}
}

func TestLoopSummaryAndLifecycle(t *testing.T) {
	m := newModel()
	m.w, m.h, m.mode = 100, 30, modeChat
	m.hub = &applyHub{}
	if !m.enterRun("converge", "PR #7", true) {
		t.Fatal("could not enter")
	}
	lp := &convergeLoop{iter: 2, max: 5, phase: "review"}
	m.loop = lp
	lp.rounds = []*roundSummary{
		{iter: 1, critical: 1, recommended: 1, notes: 2, handed: 4, closed: 3, ranFixer: true},
		{iter: 2},
	}
	if h := ansi.Strip(strings.Join(m.chatHeader(), "\n")); !strings.Contains(h, "Converge · PR #7") || !strings.Contains(h, "round 2/5 · the reviewer") {
		t.Errorf("header = %q", h)
	}
	m.loopFinish("the reviewer had nothing to add")
	if !lp.finished || lp.phase != "done" || m.rv != nil {
		t.Errorf("finish: %+v rv=%v", lp, m.rv)
	}
	last := m.log[len(m.log)-1].text
	for _, want := range []string{"Converge finished:** the reviewer had nothing to add", "round 1: the reviewer left 1 critical (!!), 1 recommended (!), 2 notes", "handed 4 threads and closed 3", "round 2: the reviewer left no comments"} {
		if !strings.Contains(last, want) {
			t.Errorf("summary lacks %q:\n%s", want, last)
		}
	}
	if h := ansi.Strip(strings.Join(m.chatHeader(), "\n")); !strings.Contains(h, "done · esc to leave") {
		t.Errorf("finished header = %q", h)
	}
	n := len(m.log)
	m.loopFinish("again") // finishing twice changes nothing
	if len(m.log) != n {
		t.Error("the summary is written once")
	}
	// Typing after the end is refused; a command still works.
	m.setErr("")
	if cmd := m.run("one more thing"); cmd != nil || !strings.Contains(m.status, "finished") {
		t.Errorf("typing in a finished loop: cmd=%v status=%q", cmd, m.status)
	}
	// Leaving the screen ends the loop for good.
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.runScr != nil || m.loop != nil {
		t.Error("esc must leave the loop screen")
	}
	if !strings.Contains(m.workspaceNotice, "/converge") {
		t.Errorf("the chat agent must hear about the run: %q", m.workspaceNotice)
	}
}

func TestStaleLoopMessagesAreIgnoredAndEscAborts(t *testing.T) {
	m := newModel()
	m.hub = &applyHub{}
	m.enterRun("converge", "PR #7", true)
	current := &convergeLoop{iter: 1, max: 3, phase: "review"}
	m.loop = current
	stale := &convergeLoop{iter: 1, max: 3}
	if cmd := m.onConvergePosted(convergePostedMsg{lp: stale}); cmd != nil || current.finished {
		t.Error("a message of another loop must be ignored")
	}
	if cmd := m.onConvergePhase(convergePhaseMsg{lp: stale}); cmd != nil || current.finished {
		t.Error("a stale phase must be ignored")
	}
	if cmd := m.onConvergeFixed(convergeFixedMsg{lp: stale}); cmd != nil || current.finished {
		t.Error("a stale fix result must be ignored")
	}
	// A failing step ends the loop with the reason.
	m.onConvergePosted(convergePostedMsg{lp: current, err: errTest("boom")})
	if !current.finished || !strings.Contains(m.log[len(m.log)-1].text, "publishing the review failed: boom") {
		t.Errorf("a failed post: %+v", current)
	}
	// Esc during a phase marks the whole loop aborted, and the end of the turn then finishes it.
	m2 := newModel()
	m2.hub = &applyHub{}
	m2.enterRun("converge", "PR #7", true)
	lp := &convergeLoop{iter: 1, max: 3, phase: "review"}
	m2.loop = lp
	rv := &reviewFlow{out: make(chan tea.Msg, 4), running: true, cancel: func() {}}
	m2.rv, m2.runScr.rv = rv, rv
	m2.reviewEsc()
	if !lp.aborted {
		t.Fatal("esc must abort the loop")
	}
	if cmd := m2.loopTurnEnded(rv); cmd != nil || !lp.finished || !strings.Contains(m2.log[len(m2.log)-1].text, "stopped by you") {
		t.Errorf("aborted loop: cmd=%v %+v", cmd, lp)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestCommandsAreBlockedInsideALoopAndRegistered(t *testing.T) {
	if findCommand("converge") == nil {
		t.Fatal("/converge is not registered")
	}
	if !runBlocked["converge"] || !runBlocked["review"] || !runBlocked["apply"] {
		t.Error("the run commands must not start inside a run")
	}
}
