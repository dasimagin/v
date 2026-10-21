package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Comments written on the PR screen wait in your pending review on GitHub
// (visible only to you) until you submit it from the header with a verdict.

// reviewSummaries are the texts of the reviews being prepared, by PR URL. A
// summary is sent with the review when it is submitted; GitHub has no pending
// form of a plain PR comment, so this is where "a comment on the PR" waits.
var reviewSummaries = map[string]string{}

func (p *workspacePicker) pending() bool { return p.pr != nil && !immediateComments }

// immediateComments is set from the settings: publish at once instead of waiting.
var immediateComments bool

// pendingCount is how many of your comments wait in the pending review.
func (d *prData) pendingCount() int {
	n := 0
	for _, f := range d.Files {
		for _, t := range f.Threads {
			for _, c := range t.Comments {
				if c.Pending && (d.Viewer == "" || c.Author == d.Viewer) {
					n++
				}
			}
		}
	}
	return n
}

// ensurePendingReview finds your pending review of the PR or starts one.
func ensurePendingReview(dir string, d *prData) (reviewReview, error) {
	if rev, ok := findPendingReview(dir, d.Repo, d.Number); ok {
		return rev, nil
	}
	if d.NodeID == "" {
		return reviewReview{}, fmt.Errorf("could not tell GitHub which PR to review")
	}
	if _, err := ghGraphQL(dir, `mutation($pr:ID!){addPullRequestReview(input:{pullRequestId:$pr}){pullRequestReview{id}}}`, map[string]any{"pr": d.NodeID}); err != nil {
		return reviewReview{}, err
	}
	rev, ok := findPendingReview(dir, d.Repo, d.Number)
	if !ok {
		return reviewReview{}, fmt.Errorf("GitHub did not start a pending review")
	}
	return rev, nil
}

// addPendingReply puts a reply to a thread into the pending review.
func addPendingReply(dir string, d *prData, thread, body string) error {
	rev, err := ensurePendingReview(dir, d)
	if err != nil {
		return err
	}
	_, err = ghGraphQL(dir, `mutation($review:ID!,$thread:ID!,$body:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewId:$review,pullRequestReviewThreadId:$thread,body:$body}){comment{id}}}`,
		map[string]any{"review": rev.NodeID, "thread": thread, "body": body})
	return err
}

// addPendingComment puts a comment on lines start..line of a file into the pending review.
func addPendingComment(dir string, d *prData, path string, start, line int, body string) error {
	rev, err := ensurePendingReview(dir, d)
	if err != nil {
		return err
	}
	card := &reviewCard{Path: path, Line: line, Start: start, Body: body, Decision: "y"}
	return addToPendingReview(dir, rev.NodeID, []*reviewCard{card})
}

// The verdicts of a submitted review, in the order of the menu.
var verdicts = []struct{ label, event string }{
	{"Comment", "COMMENT"},
	{"Approve", "APPROVE"},
	{"Request changes", "REQUEST_CHANGES"},
	{"Discard the pending review", ""},
}

type prSubmitMsg struct {
	seq  int
	text string
	err  error
}

// submitReview publishes your pending review with a verdict and the summary
// written for it, or (verdict "") deletes it.
func (m *model) submitReview(verdict int) tea.Cmd {
	p := m.picker
	if p == nil || p.pr == nil || verdict < 0 || verdict >= len(verdicts) {
		return nil
	}
	d, v := p.pr, verdicts[verdict]
	summary := strings.TrimSpace(reviewSummaries[d.URL])
	if v.event == "REQUEST_CHANGES" && summary == "" {
		m.setErr("Request changes needs a summary: write one with the Review summary button in Conversation")
		return nil
	}
	if d.pendingCount() == 0 && summary == "" && v.event != "" && v.event != "APPROVE" {
		m.flash("nothing is pending: write a comment or a summary first")
		return nil
	}
	dir := m.repo.Root
	prSeq++
	p.seq = prSeq
	seq := p.seq
	m.flash("sending to GitHub…")
	return func() tea.Msg {
		rev, ok := findPendingReview(dir, d.Repo, d.Number)
		if v.event == "" { // discard
			if !ok {
				return prSubmitMsg{seq, "", fmt.Errorf("no pending review to discard")}
			}
			_, err := ghOutput(dir, "api", "--method", "DELETE", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d", d.Repo, d.Number, rev.ID))
			return prSubmitMsg{seq, "pending review discarded", err}
		}
		if !ok { // an approval or a summary with no inline comments
			var err error
			if rev, err = ensurePendingReview(dir, d); err != nil {
				return prSubmitMsg{seq, "", err}
			}
		}
		payload := fmt.Sprintf(`{"event":%q,"body":%q}`, v.event, summary)
		_, err := ghInput(dir, []byte(payload), "api", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/events", d.Repo, d.Number, rev.ID))
		return prSubmitMsg{seq, "review submitted: " + strings.ToLower(v.label), err}
	}
}

// onPRSubmitted reloads the PR: the pending comments are published or gone.
func (m *model) onPRSubmitted(msg prSubmitMsg) tea.Cmd {
	p := m.picker
	if p == nil || p.kind != "prdiff" || p.seq != msg.seq || p.pr == nil {
		return nil
	}
	if msg.err != nil {
		m.setErr(msg.err.Error())
		return nil
	}
	delete(reviewSummaries, p.pr.URL)
	m.flash(msg.text)
	prSeq++
	p.seq = prSeq
	return reloadPR(m.repo.Root, p.pr.URL, p.seq)
}

// submitButton is the header button that opens the verdict menu.
func (m *model) submitButton() string {
	label := " Submit"
	if p := m.picker; p != nil && p.pr != nil {
		n := p.pr.pendingCount()
		if _, ok := reviewSummaries[p.pr.URL]; ok {
			n++ // the summary goes with it
		}
		if n > 0 {
			label += fmt.Sprintf(" [%d]", n)
		}
	}
	st := buttonStyle
	if m.submitMenu != nil {
		st = buttonOpen
	}
	return st.Render(label + " ▾ ")
}

func (m *model) openSubmitMenu() {
	m.repoMenu, m.menu, m.viewMenu, m.listMenu = nil, nil, nil, nil
	m.submitMenu = &viewMenu{}
}

func (m *model) submitMenuKey(k string) tea.Cmd {
	n := len(verdicts)
	switch k {
	case "up", "shift+tab":
		m.submitMenu.sel = (m.submitMenu.sel + n - 1) % n
	case "down", "tab":
		m.submitMenu.sel = (m.submitMenu.sel + 1) % n
	case "enter", " ":
		sel := m.submitMenu.sel
		m.submitMenu = nil
		return m.submitReview(sel)
	case "esc", "ctrl+s":
		m.submitMenu = nil
	}
	return nil
}

// submitMenuLayout is where the menu opens: under the Submit button, which sits left of the list button.
func (m *model) submitMenuLayout() (x, width int) {
	width = 4
	for _, v := range verdicts {
		width = max(width, lipgloss.Width(v.label)+4)
	}
	right := m.w - lipgloss.Width(m.modeLabel()) - 2 - lipgloss.Width(m.listButton()) - 1
	return max(right-lipgloss.Width(m.submitButton()), 0), width
}

func applyPRSettings(s Settings) {
	immediateComments = s.PRComments == "immediate"
	previews = map[string]previewEntry{}
}
