package main

import (
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
}

type uiComment struct {
	Author string `json:"author"`
	When   string `json:"when"`
	Body   string `json:"body"`
	State  string `json:"state,omitempty"` // a review: approved, changes requested, ...
}

type uiThread struct {
	Resolved bool        `json:"resolved"`
	Outdated bool        `json:"outdated"`
	Comments []uiComment `json:"comments"`
}

// uiDiffLine is a line of a file's patch. K is head, meta, hunk, add, del or ctx.
type uiDiffLine struct {
	K       string     `json:"k"`
	No      int        `json:"no,omitempty"` // line in the new file
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

func uiThreadOf(t prThread) uiThread {
	out := uiThread{Resolved: t.Resolved, Outdated: t.Outdated, Comments: []uiComment{}}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, uiComment{Author: c.Author, When: uiWhen(c.At), Body: c.Body})
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
	d, err := fetchPR(dir, link)
	if err != nil {
		return uiPRHead{}, err
	}
	p.mu.Lock()
	p.data = d
	p.mu.Unlock()
	h := uiPRHead{Number: d.Number, Title: d.Title, URL: d.URL, Author: d.Author, State: strings.ToLower(d.State),
		Base: d.Base, Head: d.Head, Draft: d.Draft, Labels: append([]string{}, d.Labels...), Files: []uiPRFile{}}
	for _, f := range d.Files {
		open := 0
		for _, t := range f.Threads {
			if !t.Resolved {
				open++
			}
		}
		h.Files = append(h.Files, uiPRFile{f.Path, f.Adds, f.Dels, open})
	}
	return h, nil
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
		for _, r := range d.Reviews {
			if r.State == "COMMENTED" && strings.TrimSpace(r.Body) == "" {
				continue // its inline comments are with the files
			}
			v.Reviews = append(v.Reviews, uiComment{r.Author, uiWhen(r.At), r.Body, strings.ToLower(strings.ReplaceAll(r.State, "_", " "))})
		}
		for _, c := range d.Comments {
			v.Comments = append(v.Comments, uiComment{Author: c.Author, When: uiWhen(c.At), Body: c.Body})
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
				v.Outdated = append(v.Outdated, uiThreadOf(t))
			} else {
				byLine[t.Line] = append(byLine[t.Line], uiThreadOf(t))
			}
		}
		newN, patch := 0, false
		for _, l := range f.Patch {
			line := uiDiffLine{T: l}
			switch {
			case strings.HasPrefix(l, "diff --git"):
				line.K = "head"
			case hunkPattern.MatchString(l):
				newN, _ = strconv.Atoi(hunkPattern.FindStringSubmatch(l)[2])
				patch, line.K = true, "hunk"
			case !patch:
				line.K = "meta" // index, ---, +++
			case strings.HasPrefix(l, "+"):
				line.K, line.No = "add", newN
				newN++
			case strings.HasPrefix(l, "-"):
				line.K = "del"
			default:
				line.K, line.No = "ctx", newN
				newN++
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
