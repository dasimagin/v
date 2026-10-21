package main

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// /converge runs a reviewer and a fixer in turns on one screen until the
// reviewer is satisfied. Each phase is a fresh agent:
//
//	reviewer → comments, published as a review
//	  none at all                          → done
//	fixer → fixes, pushes, replies, closes what it did
//	  a blocking comment (! or !!) was left, or the fixer left a thread open
//	                                       → the reviewer looks again
//	  else                                 → done
//	the iterations are used up             → done, with a note about what is open
type convergeLoop struct {
	iter, max int
	phase     string // "review", "fix" or "done"
	since     string // the head the previous review started from
	startHead string // the head this iteration's review started from
	handed    []string
	rounds    []*roundSummary
	aborted   bool
	finished  bool
}

type roundSummary struct {
	iter                         int
	critical, recommended, notes int
	handed, closed               int // threads given to the fixer, and how many of them it closed
	ranFixer                     bool
}

func (r *roundSummary) blocking() int { return r.critical + r.recommended }

// Messages of the loop carry its pointer, so a leftover from a loop that was
// left is ignored.
type (
	convergeReadyMsg struct {
		data *prData
		err  error
	}
	convergePostedMsg struct {
		lp  *convergeLoop
		n   int
		err error
	}
	convergePhaseMsg struct {
		lp   *convergeLoop
		data *prData
		err  error
	}
	convergeFixedMsg struct {
		lp        *convergeLoop
		data      *prData
		localHead string
		err       error
	}
)

// cmdConverge starts the loop on this branch's PR.
func (m *model) cmdConverge(string) tea.Cmd {
	switch {
	case m.settings.Agent == agentCodex:
		m.setErr("/converge needs Claude Code: pick it as the agent in /config")
		return nil
	case m.rv != nil || m.runScr != nil:
		m.setErr("a run is already open: leave it with esc first")
		return nil
	case m.busy || m.perm != nil:
		m.setErr("wait for the agent to finish first")
		return nil
	case m.repo == nil || m.repo.Branch == "" || len(m.repo.pullRequests()) == 0:
		m.setErr("no PR for this branch · /pr <url> to link one")
		return nil
	}
	pr := m.repo.selectedPR()
	if repo, _ := parsePRURL(pr.URL); repo == "" {
		m.setErr("/converge works on a github.com PR")
		return nil
	}
	if _, err := exec.LookPath("claude"); err != nil {
		m.setErr("Claude Code is not installed")
		return nil
	}
	m.note("reading " + prLabel(*pr) + "…")
	dir, url := m.repo.Root, pr.URL
	return func() tea.Msg {
		d, err := fetchPR(dir, url)
		if err == nil {
			err = checkedOutAt(dir, d)
		}
		return convergeReadyMsg{d, err}
	}
}

func (m *model) onConvergeReady(msg convergeReadyMsg) tea.Cmd {
	if msg.err != nil {
		m.noteErr(msg.err.Error())
		return nil
	}
	if !m.enterRun("converge", fmt.Sprintf("PR #%d", msg.data.Number), true) {
		return nil
	}
	n := m.settings.Iterations
	if n <= 0 {
		n = 5
	}
	m.loop = &convergeLoop{iter: 1, max: n}
	m.note(fmt.Sprintf("review and fix PR #%d in at most %d rounds, on its own · type to guide the agent at work, esc stops", msg.data.Number, n))
	return m.startReviewPhase(msg.data)
}

// freshAgent drops the running agent and its session; the screen's log stays.
func (m *model) freshAgent() {
	m.stopAgent()
	m.sessionID, m.resumeNext, m.queue = "", false, nil
	m.interrupted, m.noting, m.choice = false, false, 0
}

func (m *model) startReviewPhase(d *prData) tea.Cmd {
	lp := m.loop
	lp.phase, lp.startHead = "review", d.HeadSHA
	m.freshAgent()
	rv := &reviewFlow{url: d.URL, repo: d.Repo, number: d.Number, dir: m.repo.Root, data: d,
		auto: true, publish: true, out: make(chan tea.Msg, 64), running: true}
	m.rv, m.runScr.rv = rv, rv
	m.runScr.tools = rv.withNotes(prTools(rv))
	system, warn := renderPrompt("reviewer", m.repo.Root, promptData{Number: d.Number, Title: d.Title, Head: d.Head, Base: d.Base, Branch: m.repo.Branch,
		Auto: true, Loop: lp.iter > 1, Round: lp.iter, MaxRounds: lp.max, Since: shortSHA(lp.since)})
	m.runScr.system = system
	if warn != "" {
		m.noteErr(warn)
	}
	m.note(fmt.Sprintf("── round %d/%d · the reviewer ──", lp.iter, lp.max))
	cmd := m.prompt(fmt.Sprintf("Review pull request #%d: %s", d.Number, d.Title))
	m.armRun(rv)
	return tea.Batch(cmd, rv.wait())
}

func (m *model) startFixPhase(d *prData, refs []applyRef) tea.Cmd {
	lp := m.loop
	lp.phase = "fix"
	lp.handed = lp.handed[:0]
	for _, r := range refs {
		lp.handed = append(lp.handed, r.thread.ID)
	}
	m.freshAgent()
	rv := &reviewFlow{apply: true, url: d.URL, repo: d.Repo, number: d.Number, dir: m.repo.Root, data: d,
		auto: true, out: make(chan tea.Msg, 64), running: true}
	m.rv, m.runScr.rv = rv, rv
	m.hub.set(rv)
	m.runScr.tools = rv.withNotes(applyTools(m.hub))
	m.runScr.system = ""
	task, warn := applyPrompt(m.repo.Root, d, refs, m.repo.Branch, true, true, lp.iter, lp.max)
	if warn != "" {
		m.noteErr(warn)
	}
	m.note(fmt.Sprintf("── round %d/%d · the fixer, %d threads ──", lp.iter, lp.max, len(refs)))
	cmd := m.prompt(task)
	m.armRun(rv)
	return tea.Batch(cmd, rv.wait())
}

// loopTurnEnded is called when the agent of a phase stops; it returns what comes next.
func (m *model) loopTurnEnded(rv *reviewFlow) tea.Cmd {
	lp := m.loop
	switch {
	case lp.finished:
		return nil
	case lp.aborted:
		m.loopFinish("stopped by you")
		return nil
	}
	switch lp.phase {
	case "review":
		crit, rec, notes := rv.levelBreakdown()
		lp.rounds = append(lp.rounds, &roundSummary{iter: lp.iter, critical: crit, recommended: rec, notes: notes})
		accepted := rv.takeAccepted()
		if len(accepted) == 0 {
			m.loopFinish("the reviewer had nothing to add")
			return nil
		}
		m.flash(fmt.Sprintf("publishing %d comments to PR #%d…", len(accepted), rv.number))
		return func() tea.Msg {
			n, err := postReview(rv.dir, rv.repo, rv.number, accepted, false)
			return convergePostedMsg{lp, n, err}
		}
	case "fix":
		dir, url := rv.dir, rv.url
		return func() tea.Msg {
			d, err := fetchPR(dir, url)
			head, _ := gitOutput(dir, "rev-parse", "HEAD")
			return convergeFixedMsg{lp, d, head, err}
		}
	}
	return nil
}

// takeAccepted returns the proposals the user accepted and that are not posted yet, and marks them posted.
func (rv *reviewFlow) takeAccepted() []*reviewCard {
	rv.mu.Lock()
	defer rv.mu.Unlock()
	var out []*reviewCard
	for _, c := range rv.cards {
		if c.Decision == "y" && !c.posted {
			c.posted = true
			out = append(out, c)
		}
	}
	return out
}

// levelBreakdown counts the accepted proposals by level.
func (rv *reviewFlow) levelBreakdown() (critical, recommended, notes int) {
	rv.mu.Lock()
	defer rv.mu.Unlock()
	for _, c := range rv.cards {
		if c.Decision != "y" {
			continue
		}
		switch c.Level {
		case "critical":
			critical++
		case "recommended":
			recommended++
		case "note":
			notes++
		}
	}
	return
}

func (m *model) onConvergePosted(msg convergePostedMsg) tea.Cmd {
	lp := msg.lp
	if lp != m.loop || lp.finished {
		return nil
	}
	if lp.aborted {
		m.loopFinish("stopped by you")
		return nil
	}
	if msg.err != nil {
		m.loopFinish("publishing the review failed: " + msg.err.Error())
		return nil
	}
	dir, url := m.repo.Root, m.rv2url()
	return func() tea.Msg {
		d, err := fetchPR(dir, url)
		return convergePhaseMsg{lp, d, err}
	}
}

// rv2url is the PR of the loop.
func (m *model) rv2url() string {
	if m.runScr != nil && m.runScr.rv != nil {
		return m.runScr.rv.url
	}
	return ""
}

// onConvergePhase starts the fixer on the open threads, once the review is public.
func (m *model) onConvergePhase(msg convergePhaseMsg) tea.Cmd {
	lp := msg.lp
	if lp != m.loop || lp.finished {
		return nil
	}
	if msg.err != nil {
		m.loopFinish("reading the PR failed: " + msg.err.Error())
		return nil
	}
	refs := msg.data.unresolvedThreads()
	if len(refs) == 0 {
		m.loopFinish("no thread is open")
		return nil
	}
	if r := lp.rounds[len(lp.rounds)-1]; r != nil {
		r.handed, r.ranFixer = len(refs), true
	}
	return m.startFixPhase(msg.data, refs)
}

// onConvergeFixed decides what follows the fixer.
func (m *model) onConvergeFixed(msg convergeFixedMsg) tea.Cmd {
	lp := msg.lp
	if lp != m.loop || lp.finished {
		return nil
	}
	if msg.err != nil {
		m.loopFinish("reading the PR failed: " + msg.err.Error())
		return nil
	}
	d := msg.data
	if d.HeadSHA != "" && msg.localHead != "" && msg.localHead != d.HeadSHA {
		m.loopFinish("the fixer's commits are not on GitHub (no push), so a reviewer could not see them")
		return nil
	}
	open := 0
	handed := map[string]bool{}
	for _, id := range lp.handed {
		handed[id] = true
	}
	for _, f := range d.Files {
		for _, t := range f.Threads {
			if handed[t.ID] && !t.Resolved {
				open++
			}
		}
	}
	r := lp.rounds[len(lp.rounds)-1]
	r.closed = r.handed - open
	again, reason := loopNext(r, open, lp.iter, lp.max)
	if !again {
		m.loopFinish(reason)
		return nil
	}
	lp.iter++
	lp.since = lp.startHead
	return m.startReviewPhase(d)
}

// loopNext decides what follows a fixer: another round with the reviewer, or
// the end with the reason. The reviewer has the last word: it is asked again
// when it left a blocking comment (! or !!) or when the fixer left a thread
// open, which means the fixer waits for it; otherwise the notes were dealt
// with and nothing is left to check. Using up the rounds ends the loop either way.
func loopNext(r *roundSummary, open, iter, max int) (again bool, reason string) {
	switch {
	case r.blocking() == 0 && open == 0:
		return false, "only notes were left, and the fixer closed all the threads"
	case iter >= max:
		return false, fmt.Sprintf("the %d rounds are used up with %d threads still open", max, open)
	}
	return true, ""
}

// loopFinish ends the loop and writes its summary into the log.
func (m *model) loopFinish(reason string) {
	lp := m.loop
	if lp == nil || lp.finished {
		return
	}
	lp.finished, lp.phase = true, "done"
	m.freshAgent()
	m.rv = nil
	m.hub.set(nil)
	m.log = append(m.log, message{kind: msgAgent, text: lp.summary(reason)})
}

// summary is the report of the loop, in Markdown.
func (lp *convergeLoop) summary(reason string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Converge finished:** %s.\n\n", reason)
	for _, r := range lp.rounds {
		fmt.Fprintf(&b, "- round %d: the reviewer left %s", r.iter, describeLevels(r))
		if r.ranFixer {
			fmt.Fprintf(&b, "; the fixer was handed %d threads and closed %d", r.handed, r.closed)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func describeLevels(r *roundSummary) string {
	var parts []string
	if r.critical > 0 {
		parts = append(parts, fmt.Sprintf("%d critical (!!)", r.critical))
	}
	if r.recommended > 0 {
		parts = append(parts, fmt.Sprintf("%d recommended (!)", r.recommended))
	}
	if r.notes > 0 {
		parts = append(parts, fmt.Sprintf("%d notes", r.notes))
	}
	if len(parts) == 0 {
		return "no comments"
	}
	return strings.Join(parts, ", ")
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
