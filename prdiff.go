package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type prComment struct {
	ID           int64 // REST id, for replying
	Author, Body string
	At           time.Time
	Pending      bool // part of a review that is not submitted yet
}

type prReview struct {
	Author, State, Body string
	At                  time.Time
}

type prFile struct {
	Path    string
	Patch   []string
	Adds    int
	Dels    int
	Threads []prThread
}

// prThread is a conversation on one line of a file.
type prThread struct {
	ID       string // GraphQL node id, for replying
	Resolved bool
	Outdated bool
	Line     int // in the new file; 0 when the line is gone
	Start    int // first line of a multi-line thread, 0 for a single line
	Comments []prComment
}

// prData is a read-only snapshot of a pull request.
type prData struct {
	Repo, URL, Title, Body, Author, State string
	Number                                int
	Files                                 []prFile
	Reviews                               []prReview
	Comments                              []prComment // the conversation tab
	Base, Head, BaseSHA, HeadSHA          string
	Draft                                 bool
	Labels                                []string
	At                                    int64
}

type prDiffMsg struct {
	seq  int
	data *prData
	err  error
}

var prURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/pull/(\d+)`)

func parsePRURL(url string) (repo string, number int) {
	m := prURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", 0
	}
	number, _ = strconv.Atoi(m[2])
	return m[1], number
}

func ghOutput(dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("install gh and run gh auth login to read PRs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %s", args[0], firstOutputLine(stderr.String(), err))
	}
	return out, nil
}

// fetchPR reads the PR, its diff and its inline review comments from GitHub.
func fetchPR(dir, url string) (*prData, error) {
	repo, n := parsePRURL(url)
	if repo == "" {
		return nil, fmt.Errorf("reading a PR needs a github.com pull request link")
	}
	var view, diff, threads []byte
	var errs [3]error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		view, errs[0] = ghOutput(dir, "pr", "view", url, "--json", "number,title,body,author,state,reviews,comments,baseRefName,headRefName,baseRefOid,headRefOid,isDraft,labels")
	}()
	go func() {
		defer wg.Done()
		diff, errs[1] = ghOutput(dir, "pr", "diff", url, "--color=never")
	}()
	go func() {
		defer wg.Done()
		owner, name, _ := strings.Cut(repo, "/")
		threads, errs[2] = ghGraphQL(dir, threadsQuery, map[string]any{"owner": owner, "name": name, "n": n})
	}()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return parsePR(repo, url, view, diff, threads)
}

func parsePR(repo, url string, view, diff, threads []byte) (*prData, error) {
	var v struct {
		Number      int
		Title       string
		Body        string
		State       string
		Author      struct{ Login string }
		BaseRefName string
		HeadRefName string
		BaseRefOid  string
		HeadRefOid  string
		IsDraft     bool
		Labels      []struct{ Name string }
		Reviews     []struct {
			Author      struct{ Login string }
			State, Body string
			SubmittedAt time.Time
		}
		Comments []struct {
			Author    struct{ Login string }
			Body      string
			CreatedAt time.Time
		}
	}
	if err := json.Unmarshal(view, &v); err != nil {
		return nil, err
	}
	d := &prData{Repo: repo, URL: url, Number: v.Number, Title: v.Title, Body: v.Body, State: v.State, Author: v.Author.Login, At: time.Now().UnixNano(),
		Base: v.BaseRefName, Head: v.HeadRefName, BaseSHA: v.BaseRefOid, HeadSHA: v.HeadRefOid, Draft: v.IsDraft}
	for _, l := range v.Labels {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, r := range v.Reviews {
		d.Reviews = append(d.Reviews, prReview{r.Author.Login, r.State, r.Body, r.SubmittedAt})
	}
	for _, c := range v.Comments {
		d.Comments = append(d.Comments, prComment{Author: c.Author.Login, Body: c.Body, At: c.CreatedAt})
	}
	d.Files = splitDiff(string(diff))
	var res struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct{ Nodes []threadNode }
				}
			}
		}
	}
	if err := json.Unmarshal(threads, &res); err != nil {
		return nil, err
	}
	for _, t := range res.Data.Repository.PullRequest.ReviewThreads.Nodes {
		th := prThread{ID: t.ID, Resolved: t.IsResolved, Outdated: t.IsOutdated, Line: t.Line, Start: t.StartLine}
		for _, c := range t.Comments.Nodes {
			th.Comments = append(th.Comments, prComment{ID: c.DatabaseID, Author: c.Author.Login, Body: c.Body, At: c.CreatedAt, Pending: c.State == "PENDING"})
		}
		for i := range d.Files {
			if d.Files[i].Path == t.Path {
				d.Files[i].Threads = append(d.Files[i].Threads, th)
			}
		}
	}
	return d, nil
}

// threadsQuery reads the review threads of a PR: their state and every comment.
const threadsQuery = `query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){pullRequest(number:$n){reviewThreads(first:100){nodes{id isResolved isOutdated path line startLine comments(first:50){nodes{databaseId author{login} body createdAt state}}}}}}}`

type threadNode struct {
	ID         string
	IsResolved bool
	IsOutdated bool
	Path       string
	Line       int
	StartLine  int
	Comments   struct {
		Nodes []struct {
			DatabaseID int64
			Author     struct{ Login string }
			Body       string
			CreatedAt  time.Time
			State      string
		}
	}
}

// splitDiff cuts a unified diff into one entry per file.
func splitDiff(diff string) []prFile {
	var files []prFile
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
			path := rest
			if i := strings.LastIndex(rest, " b/"); i >= 0 {
				path = rest[i+3:]
			}
			files = append(files, prFile{Path: path})
		}
		if len(files) == 0 {
			continue
		}
		f := &files[len(files)-1]
		f.Patch = append(f.Patch, line)
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			f.Adds++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			f.Dels++
		}
	}
	return files
}

var hunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func wrapped(text string, width int) []string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if text == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(text, max(width, 10), ""), "\n")
}

// filePreview is the file's patch, with a gutter of new-file line numbers and
// each review thread under the line it belongs to. anchors holds the row of
// every thread, by its index in f.Threads.
func (d *prData) filePreview(f prFile, width int) (previewLines, []int, map[int]int, []boxHits) {
	lineAt := map[int]int{} // row of each numbered diff line -> its line in the new file
	byLine := map[int][]int{}
	var loose []int
	for i, t := range f.Threads {
		if t.Line == 0 {
			loose = append(loose, i)
			continue
		}
		byLine[t.Line] = append(byLine[t.Line], i)
	}
	anchors := make([]int, len(f.Threads))
	hits := make([]boxHits, len(f.Threads))
	// The lines a thread covers get a bar in the gutter; an open thread wins over a closed one.
	covered := map[int]bool{} // line -> thread is resolved
	for _, t := range f.Threads {
		for n := max(t.Start, 1); t.Line > 0 && n <= t.Line; n++ {
			if prev, seen := covered[n]; !seen || prev {
				covered[n] = t.Resolved
			}
		}
	}
	var out previewLines
	newN := 0
	patch := false
	pin := func(kind byte, no int) bool { return kind == 'R' && len(byLine[no]) > 0 }
	for _, l := range reflowPatch(f.Patch, diffBefore, diffAfter, pin) {
		tok := chroma.Text
		no := 0
		switch {
		case strings.HasPrefix(l, "diff --git"):
			tok = chroma.GenericHeading
		case hunkPattern.MatchString(l):
			newN, _ = strconv.Atoi(hunkPattern.FindStringSubmatch(l)[2])
			patch, tok = true, chroma.GenericSubheading
		case !patch:
			tok = chroma.Comment // index, ---, +++
		case strings.HasPrefix(l, "+"):
			tok, no = chroma.GenericInserted, newN
			newN++
		case strings.HasPrefix(l, "-"):
			tok = chroma.GenericDeleted
		default:
			no = newN
			newN++
		}
		// number, a space, the bar of the lines a thread covers, a space: the
		// bar must not touch the number or the code
		num := "    "
		if no > 0 {
			num = fmt.Sprintf("%4d", no)
		}
		bar, barTok := " ", chroma.Comment
		if resolved, ok := covered[no]; ok && no > 0 {
			bar, barTok = "▎", chroma.LiteralString
			if resolved {
				barTok = chroma.Comment
			}
		}
		line := []seg{{num + " ", chroma.Comment}, {bar, barTok}, {" ", chroma.Comment}}
		if l != "" {
			line = append(line, seg{l, tok})
		}
		if no > 0 {
			lineAt[len(out)] = no
		}
		out = append(out, line)
		for _, i := range byLine[no] {
			anchors[i] = len(out)
			box, h := threadBox(f.Threads[i], width)
			out = append(out, box...)
			hits[i] = h
		}
	}
	if len(loose) > 0 {
		out = append(out, nil, []seg{{"Outdated", chroma.Comment}})
		for _, i := range loose {
			anchors[i] = len(out)
			box, h := threadBox(f.Threads[i], width)
			out = append(out, box...)
			hits[i] = h
		}
	}
	return out, anchors, lineAt, hits
}

// conversationPreview is the PR itself: description, reviews and general comments.
func (d *prData) conversationPreview(width int) previewLines {
	state := strings.ToLower(d.State)
	if d.Draft {
		state += ", draft"
	}
	title := bg.Render("  ") + fgStyle.Bold(true).Render(d.Title)
	button := commentButton.Render(" New comment ")
	title += bg.Render(strings.Repeat(" ", max(width-1-lipgloss.Width(title)-lipgloss.Width(button), 1))) + button
	out := previewLines{
		rawLine(title),
		rawLine(bg.Render("  ") + hintStyle.Render(fmt.Sprintf("#%d by @%s · %s · %s ← %s · %d files", d.Number, d.Author, state, d.Base, d.Head, len(d.Files)))),
		rawLine(bg.Render("  ") + hintStyle.Render(d.URL)),
		nil,
	}
	for _, l := range markdownBody(d.Body, width-2) {
		out = append(out, rawLine(bg.Render("  ")+l))
	}
	for _, r := range d.Reviews {
		if r.State == "COMMENTED" && strings.TrimSpace(r.Body) == "" {
			continue // its inline comments are shown with the files
		}
		color := hintStyle
		switch r.State {
		case "APPROVED":
			color = bg.Foreground(lipgloss.Color(cur.Added))
		case "CHANGES_REQUESTED":
			color = bg.Foreground(lipgloss.Color(cur.Deleted))
		}
		label := strings.ToLower(strings.ReplaceAll(r.State, "_", " "))
		out = append(out, nil)
		box, _ := messageBox([]boxEntry{{author: r.Author, at: r.At, body: r.Body, badges: []badge{{label, color}}}}, false, width, "", "")
		out = append(out, box...)
	}
	for _, c := range d.Comments {
		out = append(out, nil)
		box, _ := messageBox([]boxEntry{{author: c.Author, at: c.At, body: c.Body}}, false, width, "", "")
		out = append(out, box...)
	}
	return out
}

// prAnchors remembers where each thread sits in a cached file preview, and
// prLineRows where each numbered diff line does.
var (
	prAnchors  = map[string][]int{}
	prLineRows = map[string]map[int]int{}
	prBoxHits  = map[string][]boxHits{}
)

// prHit is a clickable spot in the preview: columns [x0, x1) of a row.
type prHit struct {
	x0, x1 int
	kind   string // "reply", "resolve", "line" or "general"
	file   int
	thread int
	line   int
}

// prPreview builds the right pane for one list item: "" is the conversation,
// "summary" the draft review text, "f:c" comment c of file f, else a path.
// For a comment, p.focus is the row to scroll to.
func (p *workspacePicker) prPreview(value string, width int) previewLines {
	d := p.pr
	p.focus = 0
	p.hits, p.rowLines = map[int][]prHit{}, nil
	if d == nil {
		return nil
	}
	fileAt, commentAt := -1, -1
	if a, b, ok := strings.Cut(value, ":"); ok {
		fileAt, _ = strconv.Atoi(a)
		commentAt, _ = strconv.Atoi(b)
	} else if value != "" {
		for i, f := range d.Files {
			if f.Path == value {
				fileAt = i
			}
		}
	}
	key := fmt.Sprintf("pr:%d:%s:%d:%d:%d:%d", d.At, value, width, d.Number, diffBefore, diffAfter)
	if fileAt >= 0 && fileAt < len(d.Files) {
		f := d.Files[fileAt]
		lines := cachedPreview(key, 0, func() previewLines {
			lines, anchors, lineAt, hits := d.filePreview(f, width)
			prAnchors[key], prLineRows[key], prBoxHits[key] = anchors, lineAt, hits
			return lines
		})
		anchors := prAnchors[key]
		if commentAt >= 0 && commentAt < len(anchors) {
			p.focus = anchors[commentAt]
		}
		p.rowLines = prLineRows[key]
		for row, line := range prLineRows[key] {
			p.hits[row] = append(p.hits[row], prHit{x0: 0, x1: width, kind: "line", file: fileAt, line: line})
		}
		for ti, row := range anchors {
			if t, h := f.Threads[ti], prBoxHits[key][ti]; t.ID != "" {
				p.hits[row] = append(p.hits[row], prHit{x0: h.state[0], x1: h.state[1], kind: "resolve", file: fileAt, thread: ti})
				for bi, kind := range []string{"reply", "apply"} {
					if bi < len(h.btn) {
						p.hits[row] = append(p.hits[row], prHit{x0: h.btn[bi][0], x1: h.btn[bi][1], kind: kind, file: fileAt, thread: ti})
					}
				}
			}
		}
		return lines
	}
	button := lipgloss.Width(commentButton.Render(" New comment "))
	p.hits[0] = []prHit{{x0: width - 1 - button, x1: width - 1, kind: "general"}}
	return cachedPreview(key, 0, func() previewLines {
		return d.conversationPreview(width)
	})
}
