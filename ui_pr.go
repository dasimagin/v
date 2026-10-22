package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// uiPRFile is a row of the PR's file list.
type uiPRFile struct {
	Path    string `json:"path"`
	Adds    int    `json:"adds"`
	Dels    int    `json:"dels"`
	Threads int    `json:"threads"` // open review threads
}

// uiPRHead is the PR as the screen opens: who, what, and its files.
type uiPRHead struct {
	Number int        `json:"number"`
	Title  string     `json:"title"`
	URL    string     `json:"url"`
	Author string     `json:"author"`
	State  string     `json:"state"`
	Base   string     `json:"base"`
	Head   string     `json:"head"`
	Draft  bool       `json:"draft"`
	Labels []string   `json:"labels"`
	Files  []uiPRFile `json:"files"`

	Viewer    string        `json:"viewer"`    // your login: your comments can be deleted
	Pending   int           `json:"pending"`   // your comments waiting in the pending review
	Summary   string        `json:"summary"`   // the review summary written so far
	Immediate bool          `json:"immediate"` // comments are published at once, not collected in a review
	Threads   []uiThreadRef `json:"threads"`   // every review thread, for the Comments list
	Talk      int           `json:"talk"`      // comments in the conversation
}

// uiThreadRef is a row of the Comments list: one review thread and where it is.
type uiThreadRef struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Resolved bool   `json:"resolved"`
	Outdated bool   `json:"outdated"`
	Author   string `json:"author"` // of the first comment
	Preview  string `json:"preview"`
	Count    int    `json:"count"`
	Pending  bool   `json:"pending"`
}

type uiComment struct {
	Author  string `json:"author"`
	When    string `json:"when"`
	Body    string `json:"body"`
	State   string `json:"state,omitempty"` // a review: approved, changes requested, ...
	ID      string `json:"id,omitempty"`    // to delete it
	Mine    bool   `json:"mine,omitempty"`  // yours: it can be deleted
	Pending bool   `json:"pending,omitempty"`
}

type uiThread struct {
	ID       string      `json:"id"`
	Resolved bool        `json:"resolved"`
	Outdated bool        `json:"outdated"`
	Path     string      `json:"path"`
	Line     int         `json:"line"`
	Comments []uiComment `json:"comments"`
}

// uiDiffLine is a line of a file's patch. K is head, meta, hunk, add, del or ctx.
type uiDiffLine struct {
	K       string     `json:"k"`
	No      int        `json:"no,omitempty"`  // line in the new file
	Old     int        `json:"old,omitempty"` // line in the old file (context and removed lines)
	T       string     `json:"t"`
	Threads []uiThread `json:"threads,omitempty"`
}

// uiPRView is the right pane: the conversation, or one file's diff.
type uiPRView struct {
	Body     string       `json:"body"`
	Reviews  []uiComment  `json:"reviews"`
	Comments []uiComment  `json:"comments"`
	Lines    []uiDiffLine `json:"lines"`
	Outdated []uiThread   `json:"outdated"`
	Summary  string       `json:"summary,omitempty"` // the review summary written so far
}

// uiPRs holds the PR that is open on the screen.
type uiPRs struct {
	mu   sync.Mutex
	data *prData
}

func uiWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return ago(t)
}

func uiCommentOf(c prComment, viewer string) uiComment {
	return uiComment{Author: c.Author, When: uiWhen(c.At), Body: c.Body, ID: c.NodeID, Mine: viewer != "" && c.Author == viewer, Pending: c.Pending}
}

func uiThreadOf(t prThread, path, viewer string) uiThread {
	out := uiThread{ID: t.ID, Resolved: t.Resolved, Outdated: t.Outdated, Path: path, Line: t.Line, Comments: []uiComment{}}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, uiCommentOf(c, viewer))
	}
	return out
}

// uiPRURL finds the PR to open: arg is a link or a number; with none, the
// open PR of the current branch.
func uiPRURL(dir, arg string) (string, error) {
	r := readRepository(dir)
	if arg != "" {
		if n, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && n > 0 {
			if r == nil {
				return "", fmt.Errorf("workdir is not in a Git repository")
			}
			repo := githubRepository(r.Target)
			if repo == "" {
				return "", fmt.Errorf("use a full PR URL for this remote")
			}
			return fmt.Sprintf("https://github.com/%s/pull/%d", repo, n), nil
		}
		if u, err := url.Parse(arg); err != nil || u.Scheme != "https" || u.Host == "" {
			return "", fmt.Errorf("usage: /pr <URL or number>")
		}
		return arg, nil
	}
	if r == nil || r.Branch == "" {
		return "", fmt.Errorf("select a Git branch first, or give /pr a link or a number")
	}
	if r.Link != "" {
		return r.Link, nil
	}
	prs, err := discoverPullRequests(*r)
	if err != nil {
		return "", err
	}
	if len(prs) == 0 {
		return "", fmt.Errorf("no open PR for this branch · /pr <url or number>")
	}
	return prs[0].URL, nil
}

// Load reads a PR from GitHub (with gh) and makes it the open one.
func (p *uiPRs) Load(dir, arg string) (uiPRHead, error) {
	link, err := uiPRURL(dir, arg)
	if err != nil {
		return uiPRHead{}, err
	}
	applyPRSettings(loadedSettings())
	d, err := fetchPR(dir, link)
	if err != nil {
		return uiPRHead{}, err
	}
	p.mu.Lock()
	p.data = d
	p.mu.Unlock()
	return uiHeadOf(d), nil
}

// uiHeadOf is what the screen needs to list a PR: its facts, files and threads.
func uiHeadOf(d *prData) uiPRHead {
	h := uiPRHead{Number: d.Number, Title: d.Title, URL: d.URL, Author: d.Author, State: strings.ToLower(d.State),
		Base: d.Base, Head: d.Head, Draft: d.Draft, Labels: append([]string{}, d.Labels...), Files: []uiPRFile{},
		Viewer: d.Viewer, Pending: d.pendingCount(), Summary: reviewSummaries[d.URL], Immediate: immediateComments,
		Threads: []uiThreadRef{}, Talk: len(d.Comments)}
	for _, f := range d.Files {
		open := 0
		for _, t := range f.Threads {
			if !t.Resolved {
				open++
			}
			ref := uiThreadRef{ID: t.ID, Path: f.Path, Line: t.Line, Resolved: t.Resolved, Outdated: t.Outdated, Count: len(t.Comments)}
			if len(t.Comments) > 0 {
				ref.Author = t.Comments[0].Author
				ref.Preview = strings.SplitN(strings.TrimSpace(t.Comments[0].Body), "\n", 2)[0]
				for _, c := range t.Comments {
					ref.Pending = ref.Pending || c.Pending
				}
			}
			h.Threads = append(h.Threads, ref)
		}
		h.Files = append(h.Files, uiPRFile{f.Path, f.Adds, f.Dels, open})
	}
	return h
}

// View is the conversation (path "") or the diff of one file of the open PR.
func (p *uiPRs) View(path string) uiPRView {
	p.mu.Lock()
	d := p.data
	p.mu.Unlock()
	v := uiPRView{Reviews: []uiComment{}, Comments: []uiComment{}, Lines: []uiDiffLine{}, Outdated: []uiThread{}}
	if d == nil {
		return v
	}
	if path == "" {
		v.Body = d.Body
		v.Summary = reviewSummaries[d.URL]
		for _, r := range d.Reviews {
			if r.State == "COMMENTED" && strings.TrimSpace(r.Body) == "" {
				continue // its inline comments are with the files
			}
			v.Reviews = append(v.Reviews, uiComment{Author: r.Author, When: uiWhen(r.At), Body: r.Body, State: strings.ToLower(strings.ReplaceAll(r.State, "_", " "))})
		}
		for _, c := range d.Comments {
			v.Comments = append(v.Comments, uiCommentOf(c, d.Viewer))
		}
		return v
	}
	for _, f := range d.Files {
		if f.Path != path {
			continue
		}
		byLine := map[int][]uiThread{}
		for _, t := range f.Threads {
			if t.Line == 0 {
				v.Outdated = append(v.Outdated, uiThreadOf(t, f.Path, d.Viewer))
			} else {
				byLine[t.Line] = append(byLine[t.Line], uiThreadOf(t, f.Path, d.Viewer))
			}
		}
		oldN, newN, patch := 0, 0, false
		// The patch from GitHub has its own context; it can be trimmed to the settings, not widened.
		pin := func(kind byte, no int) bool { return kind == 'R' && len(byLine[no]) > 0 }
		for _, l := range reflowPatch(f.Patch, diffBefore, diffAfter, pin) {
			line := uiDiffLine{T: l}
			switch {
			case strings.HasPrefix(l, "diff --git"):
				line.K = "head"
			case hunkPattern.MatchString(l):
				hm := hunkPattern.FindStringSubmatch(l)
				oldN, _ = strconv.Atoi(hm[1])
				newN, _ = strconv.Atoi(hm[2])
				patch, line.K = true, "hunk"
			case !patch:
				line.K = "meta" // index, ---, +++
			case strings.HasPrefix(l, "+"):
				line.K, line.No = "add", newN
				newN++
			case strings.HasPrefix(l, "-"):
				line.K, line.Old = "del", oldN
				oldN++
			default:
				line.K, line.No, line.Old = "ctx", newN, oldN
				newN++
				oldN++
			}
			if line.No > 0 {
				line.Threads = byLine[line.No]
			}
			v.Lines = append(v.Lines, line)
		}
		break
	}
	return v
}

func loadedSettings() Settings {
	s, _ := loadSettings()
	return s
}

// The actions on the open PR. Each one does its work on GitHub, reads the PR again
// and returns its new head; the page then asks for the view it is showing.

func (p *uiPRs) open() (*prData, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.data == nil {
		return nil, fmt.Errorf("no PR is open")
	}
	return p.data, nil
}

// refresh reads the PR again, so that the new state of its comments shows.
func (p *uiPRs) refresh(dir string, d *prData) (uiPRHead, error) {
	n, err := fetchPR(dir, d.URL)
	if err != nil {
		return uiPRHead{}, err
	}
	p.mu.Lock()
	p.data = n
	p.mu.Unlock()
	return uiHeadOf(n), nil
}

// threadByID finds a review thread and the file it is in.
func threadByID(d *prData, id string) (prFile, prThread, bool) {
	for _, f := range d.Files {
		for _, t := range f.Threads {
			if t.ID == id {
				return f, t, true
			}
		}
	}
	return prFile{}, prThread{}, false
}

// Reply answers a review thread: in the pending review, or at once.
func (p *uiPRs) Reply(dir, thread, body string) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	body = strings.TrimSpace(body)
	_, t, ok := threadByID(d, thread)
	if !ok || body == "" || len(t.Comments) == 0 {
		return uiPRHead{}, fmt.Errorf("nothing to reply to")
	}
	if immediateComments {
		_, err = ghOutput(dir, "api", "--method", "POST", fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", d.Repo, d.Number, t.Comments[0].ID), "-f", "body="+body)
	} else {
		err = addPendingReply(dir, d, thread, body)
	}
	if err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// Comment puts a comment on lines start..line of a file (start 0: just line).
func (p *uiPRs) Comment(dir, path string, start, line int, body string) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" || line <= 0 {
		return uiPRHead{}, fmt.Errorf("nothing to comment")
	}
	if start <= 0 || start > line {
		start = line
	}
	var file *prFile
	for i := range d.Files {
		if d.Files[i].Path == path {
			file = &d.Files[i]
		}
	}
	if file == nil || !file.rangeOK(start, line) {
		return uiPRHead{}, fmt.Errorf("lines %d–%d are not one stretch of the diff of %s (a comment range must stay inside one hunk)", start, line, path)
	}
	if immediateComments {
		review := map[string]any{"body": body, "commit_id": d.HeadSHA, "path": path, "line": line, "side": "RIGHT"}
		if start < line {
			review["start_line"], review["start_side"] = start, "RIGHT"
		}
		b, _ := json.Marshal(review)
		_, err = ghInput(dir, b, "api", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/comments", d.Repo, d.Number))
	} else {
		err = addPendingComment(dir, d, path, start, line, body)
	}
	if err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// Talk adds a comment to the PR's conversation, at once. With summary set (and comments
// being collected in a review) the text is kept as the summary sent with the review.
func (p *uiPRs) Talk(dir, body string, summary bool) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	body = strings.TrimSpace(body)
	if summary {
		if body == "" {
			delete(reviewSummaries, d.URL)
		} else {
			reviewSummaries[d.URL] = body
		}
		return uiHeadOf(d), nil
	}
	if body == "" {
		return uiPRHead{}, fmt.Errorf("nothing to comment")
	}
	if _, err := ghOutput(dir, "api", "--method", "POST", fmt.Sprintf("repos/%s/issues/%d/comments", d.Repo, d.Number), "-f", "body="+body); err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// Resolve resolves a review thread or opens it again.
func (p *uiPRs) Resolve(dir, thread string, resolved bool) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	query := `mutation($id:ID!){unresolveReviewThread(input:{threadId:$id}){thread{id}}}`
	if resolved {
		query = `mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{id}}}`
	}
	if _, err := ghGraphQL(dir, query, map[string]any{"id": thread}); err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// Delete removes one of your comments: a review comment, or one of the conversation.
func (p *uiPRs) Delete(dir, id string, conversation bool) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	query := `mutation($id:ID!){deletePullRequestReviewComment(input:{id:$id}){clientMutationId}}`
	if conversation {
		query = `mutation($id:ID!){deleteIssueComment(input:{id:$id}){clientMutationId}}`
	}
	if _, err := ghGraphQL(dir, query, map[string]any{"id": id}); err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// Submit publishes your pending review with a verdict (an index into verdicts) and a
// summary, or discards it.
func (p *uiPRs) Submit(dir string, verdict int, summary string) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	if verdict < 0 || verdict >= len(verdicts) {
		return uiPRHead{}, fmt.Errorf("unknown verdict")
	}
	v := verdicts[verdict]
	summary = strings.TrimSpace(summary)
	if v.event == "REQUEST_CHANGES" && summary == "" {
		return uiPRHead{}, fmt.Errorf("requesting changes needs a summary")
	}
	if d.pendingCount() == 0 && summary == "" && v.event != "" && v.event != "APPROVE" {
		return uiPRHead{}, fmt.Errorf("nothing is pending: write a comment or a summary first")
	}
	rev, ok := findPendingReview(dir, d.Repo, d.Number)
	if v.event == "" { // discard
		if !ok {
			return uiPRHead{}, fmt.Errorf("no pending review to discard")
		}
		if _, err := ghOutput(dir, "api", "--method", "DELETE", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d", d.Repo, d.Number, rev.ID)); err != nil {
			return uiPRHead{}, err
		}
	} else {
		if !ok { // an approval or a summary with no inline comments
			if rev, err = ensurePendingReview(dir, d); err != nil {
				return uiPRHead{}, err
			}
		}
		payload, _ := json.Marshal(map[string]string{"event": v.event, "body": summary})
		if _, err := ghInput(dir, payload, "api", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/events", d.Repo, d.Number, rev.ID)); err != nil {
			return uiPRHead{}, err
		}
	}
	delete(reviewSummaries, d.URL)
	return p.refresh(dir, d)
}

// Edit changes the title and the description of the PR.
func (p *uiPRs) Edit(dir, title, body string) (uiPRHead, error) {
	d, err := p.open()
	if err != nil {
		return uiPRHead{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return uiPRHead{}, fmt.Errorf("the title cannot be empty")
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	if _, err := ghInput(dir, payload, "api", "--method", "PATCH", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d", d.Repo, d.Number)); err != nil {
		return uiPRHead{}, err
	}
	return p.refresh(dir, d)
}

// uiPRRef is a pull request of the current branch, for the chip beside the branch.
type uiPRRef struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

// uiBranchPRs finds the PRs of the branch dir is on: the one linked to it, and the open ones GitHub knows.
// It asks GitHub, so it is slow and the page calls it on its own, off the main path.
func uiBranchPRs(dir string) []uiPRRef {
	out := []uiPRRef{}
	r := readRepository(dir)
	if r == nil || r.Branch == "" {
		return out
	}
	if found, err := discoverPullRequests(*r); err == nil {
		r.PRs = found
	}
	for _, p := range r.pullRequests() {
		out = append(out, uiPRRef{p.Number, p.Title, p.URL, strings.ToLower(p.State)})
	}
	return out
}

// uiCheck is one CI check of the PR: bucket is pass, fail, pending, skipping or cancel.
type uiCheck struct {
	Name        string `json:"name"`
	Workflow    string `json:"workflow"`
	Bucket      string `json:"bucket"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

// Checks asks GitHub for the CI checks of the open PR.
func (p *uiPRs) Checks(dir string) ([]uiCheck, error) {
	d, err := p.open()
	if err != nil {
		return nil, err
	}
	found, err := fetchChecks(dir, d.URL)
	if err != nil {
		return nil, err
	}
	out := make([]uiCheck, 0, len(found))
	for _, c := range found {
		out = append(out, uiCheck{c.Name, c.Workflow, c.Bucket, c.Link, c.Description})
	}
	return out, nil
}
