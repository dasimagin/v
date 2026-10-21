package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
)

type prComment struct {
	Author, Body string
	At           time.Time
	Line         int    // line in the new file (RIGHT) or the old one (LEFT); 0 if outdated
	Side         string // "RIGHT" or "LEFT"
	Draft        bool   // written by the reviewer here, not posted yet
}

type prReview struct {
	Author, State, Body string
	At                  time.Time
}

type prFile struct {
	Path     string
	Patch    []string
	Adds     int
	Dels     int
	Comments []prComment
}

// prData is a read-only snapshot of a pull request.
type prData struct {
	Repo, URL, Title, Body, Author, State string
	Number                                int
	Files                                 []prFile
	Reviews                               []prReview
	Comments                              []prComment // the conversation tab
	Summary                               string      // draft review text
	Notes                                 []string    // drafts that do not sit on a diff line
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
	var view, diff, inline []byte
	var errs [3]error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		view, errs[0] = ghOutput(dir, "pr", "view", url, "--json", "number,title,body,author,state,reviews,comments")
	}()
	go func() {
		defer wg.Done()
		diff, errs[1] = ghOutput(dir, "pr", "diff", url, "--color=never")
	}()
	go func() {
		defer wg.Done()
		inline, errs[2] = ghOutput(dir, "api", "--paginate", "--jq", ".[]", fmt.Sprintf("repos/%s/pulls/%d/comments", repo, n))
	}()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return parsePR(repo, url, view, diff, inline)
}

func parsePR(repo, url string, view, diff, inline []byte) (*prData, error) {
	var v struct {
		Number  int
		Title   string
		Body    string
		State   string
		Author  struct{ Login string }
		Reviews []struct {
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
	d := &prData{Repo: repo, URL: url, Number: v.Number, Title: v.Title, Body: v.Body, State: v.State, Author: v.Author.Login, At: time.Now().UnixNano()}
	for _, r := range v.Reviews {
		d.Reviews = append(d.Reviews, prReview{r.Author.Login, r.State, r.Body, r.SubmittedAt})
	}
	for _, c := range v.Comments {
		d.Comments = append(d.Comments, prComment{Author: c.Author.Login, Body: c.Body, At: c.CreatedAt})
	}
	d.Files = splitDiff(string(diff))
	dec := json.NewDecoder(bytes.NewReader(inline))
	for {
		var c struct {
			Path         string
			Line         int
			OriginalLine int `json:"original_line"`
			Side         string
			Body         string
			CreatedAt    time.Time `json:"created_at"`
			User         struct{ Login string }
		}
		if err := dec.Decode(&c); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		for i := range d.Files {
			if d.Files[i].Path == c.Path {
				d.Files[i].Comments = append(d.Files[i].Comments, prComment{Author: c.User.Login, Body: c.Body, At: c.CreatedAt, Line: c.Line, Side: c.Side})
			}
		}
	}
	return d, nil
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

func commentLines(c prComment, width int) previewLines {
	var out previewLines
	if c.Draft {
		out = append(out, []seg{{"  ┃ ", chroma.LiteralString}, {"draft by Claude", chroma.NameClass}, {" · not posted", chroma.Comment}})
	} else {
		out = append(out, []seg{{"  ┃ ", chroma.LiteralString}, {"@" + c.Author, chroma.NameClass}, {" · " + ago(c.At), chroma.Comment}})
	}
	for _, l := range wrapped(c.Body, width-4) {
		out = append(out, []seg{{"  ┃ ", chroma.LiteralString}, {l, chroma.Text}})
	}
	return out
}

// filePreview is the file's patch with its review comments under the lines they belong to.
func (d *prData) filePreview(f prFile, width int) previewLines {
	byLine := map[string][]prComment{}
	var loose []prComment
	for _, c := range f.Comments {
		if c.Line == 0 {
			loose = append(loose, c)
			continue
		}
		byLine[c.Side+strconv.Itoa(c.Line)] = append(byLine[c.Side+strconv.Itoa(c.Line)], c)
	}
	var out previewLines
	oldN, newN := 0, 0
	patch := false
	pin := func(kind byte, no int) bool {
		side := "RIGHT"
		if kind == 'L' {
			side = "LEFT"
		}
		return len(byLine[side+strconv.Itoa(no)]) > 0
	}
	for _, l := range reflowPatch(f.Patch, diffBefore, diffAfter, pin) {
		tok := chroma.Text
		key := ""
		switch {
		case strings.HasPrefix(l, "diff --git"):
			tok = chroma.GenericHeading
		case hunkPattern.MatchString(l):
			m := hunkPattern.FindStringSubmatch(l)
			oldN, _ = strconv.Atoi(m[1])
			newN, _ = strconv.Atoi(m[2])
			patch, tok = true, chroma.GenericSubheading
		case !patch:
			tok = chroma.Comment // index, ---, +++
		case strings.HasPrefix(l, "+"):
			tok, key = chroma.GenericInserted, "RIGHT"+strconv.Itoa(newN)
			newN++
		case strings.HasPrefix(l, "-"):
			tok, key = chroma.GenericDeleted, "LEFT"+strconv.Itoa(oldN)
			oldN++
		default:
			key = "RIGHT" + strconv.Itoa(newN)
			oldN++
			newN++
		}
		var segs []seg
		if l != "" {
			segs = []seg{{l, tok}}
		}
		out = append(out, segs)
		for _, c := range byLine[key] {
			out = append(out, commentLines(c, width)...)
		}
	}
	if len(loose) > 0 {
		out = append(out, nil, []seg{{"Outdated comments", chroma.Comment}})
		for _, c := range loose {
			out = append(out, commentLines(c, width)...)
		}
	}
	return out
}

// conversationPreview is the PR itself: description, reviews and general comments.
func (d *prData) conversationPreview(width int) previewLines {
	state := strings.ToLower(d.State)
	out := previewLines{
		{{d.Title, chroma.GenericHeading}},
		{{fmt.Sprintf("#%d by @%s · %s · %d files", d.Number, d.Author, state, len(d.Files)), chroma.Comment}},
		{{d.URL, chroma.Comment}},
		nil,
	}
	for _, l := range wrapped(d.Body, width-2) {
		out = append(out, []seg{{l, chroma.Text}})
	}
	if d.Summary != "" || len(d.Notes) > 0 {
		out = append(out, nil, []seg{{"Draft review by Claude · not posted", chroma.NameClass}})
		for _, l := range wrapped(d.Summary, width-2) {
			out = append(out, []seg{{l, chroma.Text}})
		}
		for _, n := range d.Notes {
			out = append(out, nil)
			for _, l := range wrapped(n, width-4) {
				out = append(out, []seg{{"  ┃ ", chroma.LiteralString}, {l, chroma.Text}})
			}
		}
	}
	for _, r := range d.Reviews {
		if r.State == "COMMENTED" && strings.TrimSpace(r.Body) == "" {
			continue // its inline comments are shown with the files
		}
		out = append(out, nil, []seg{{"@" + r.Author, chroma.NameClass}, {" " + strings.ToLower(strings.ReplaceAll(r.State, "_", " ")) + " · " + ago(r.At), chroma.Comment}})
		for _, l := range wrapped(r.Body, width-4) {
			out = append(out, []seg{{"  " + l, chroma.Text}})
		}
	}
	for _, c := range d.Comments {
		out = append(out, nil)
		out = append(out, commentLines(c, width)...)
	}
	return out
}

// prPreview builds the right pane for one list item; path "" is the conversation.
func (p *workspacePicker) prPreview(path string, width int) previewLines {
	d := p.pr
	if d == nil {
		return nil
	}
	key := fmt.Sprintf("pr:%d:%s:%d:%d:%d:%d", d.At, path, width, d.Number, diffBefore, diffAfter)
	return cachedPreview(key, 0, func() previewLines {
		if path == "" {
			return d.conversationPreview(width)
		}
		for _, f := range d.Files {
			if f.Path == path {
				return d.filePreview(f, width)
			}
		}
		return nil
	})
}
