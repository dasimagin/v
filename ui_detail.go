package main

import (
	"fmt"
	"strconv"
	"strings"
)

// uiDiffRow is one line of a patch: K is "add", "del", "ctx" or "hunk".
type uiDiffRow struct {
	K   string `json:"k"`
	Old int    `json:"old"`
	New int    `json:"new"`
	T   string `json:"t"`
}

type uiFileDiff struct {
	Path   string      `json:"path"`
	Add    int         `json:"add"`
	Del    int         `json:"del"`
	Binary bool        `json:"binary"`
	More   int         `json:"more"` // rows left out
	Rows   []uiDiffRow `json:"rows"`
}

// uiCommitDetail is what the /show pane draws: the message, the files and the patch.
type uiCommitDetail struct {
	Kind      string       `json:"kind"`
	Hash      string       `json:"hash"`
	Short     string       `json:"short"`
	Author    string       `json:"author"`
	Email     string       `json:"email"`
	Date      string       `json:"date"`
	Rel       string       `json:"rel"`
	Subject   string       `json:"subject"`
	Body      string       `json:"body"`
	Refs      []logRef     `json:"refs"`
	Files     []uiFileDiff `json:"files"`
	MoreFiles int          `json:"moreFiles"`
	Err       string       `json:"err"`
}

const (
	uiDetailFiles = 40
	uiDetailRows  = 300
)

func uiCommitOf(dir, hash string) uiCommitDetail {
	d := uiCommitDetail{Kind: "commit", Hash: hash, Files: []uiFileDiff{}, Refs: []logRef{}}
	const sep = "\x1f"
	out, err := gitOutput(dir, "show", "-s", "--date=format:%Y-%m-%d %H:%M", "--format=%H"+sep+"%h"+sep+"%an"+sep+"%ae"+sep+"%ad"+sep+"%cr"+sep+"%s"+sep+"%D"+sep+"%b", hash, "--")
	if err != nil {
		d.Err = err.Error()
		return d
	}
	f := strings.SplitN(out, sep, 9)
	if len(f) < 9 {
		d.Err = "unexpected output from git"
		return d
	}
	d.Refs = parseRefs(f[7], gitRemotes(dir))
	d.Hash, d.Short, d.Author, d.Email, d.Date, d.Rel, d.Subject, d.Body = f[0], f[1], f[2], f[3], f[4], f[5], f[6], strings.TrimSpace(f[8])
	patch, err := gitOutput(dir, "show", "--format=", "--no-color", fmt.Sprintf("-U%d", max(diffBefore, diffAfter)), hash, "--")
	if err != nil {
		d.Err = err.Error()
		return d
	}
	// Git gives as much context as the larger of the two settings; each side is then trimmed to its own.
	d.Files, d.MoreFiles = parsePatch(strings.Join(reflowPatch(strings.Split(patch, "\n"), diffBefore, diffAfter, nil), "\n"))
	return d
}

// parsePatch splits a unified diff into files; each keeps at most uiDetailRows rows.
func parsePatch(patch string) ([]uiFileDiff, int) {
	files := []uiFileDiff{}
	more := 0
	var cur *uiFileDiff
	skip := false
	old, nw := 0, 0
	for _, l := range strings.Split(patch, "\n") {
		if rest, ok := strings.CutPrefix(l, "diff --git "); ok {
			skip = false
			if len(files) == uiDetailFiles {
				more++
				skip, cur = true, nil
				continue
			}
			path := rest
			if i := strings.LastIndex(rest, " b/"); i >= 0 {
				path = rest[i+3:]
			}
			files = append(files, uiFileDiff{Path: path, Rows: []uiDiffRow{}})
			cur = &files[len(files)-1]
			continue
		}
		if skip || cur == nil {
			continue
		}
		switch {
		case strings.HasPrefix(l, "Binary files"), strings.HasPrefix(l, "GIT binary patch"):
			cur.Binary = true
		case strings.HasPrefix(l, "@@"):
			// @@ -a,b +c,d @@ context
			parts := strings.Fields(l)
			if len(parts) >= 3 {
				old = hunkStart(parts[1])
				nw = hunkStart(parts[2])
			}
			cur.row(uiDiffRow{K: "hunk", T: l})
		case old == 0 && nw == 0:
			// file header lines before the first hunk
		case strings.HasPrefix(l, "+"):
			cur.Add++
			cur.row(uiDiffRow{K: "add", New: nw, T: l[1:]})
			nw++
		case strings.HasPrefix(l, "-"):
			cur.Del++
			cur.row(uiDiffRow{K: "del", Old: old, T: l[1:]})
			old++
		case strings.HasPrefix(l, "\\"):
		default:
			t := strings.TrimPrefix(l, " ")
			cur.row(uiDiffRow{K: "ctx", Old: old, New: nw, T: t})
			old++
			nw++
		}
	}
	return files, more
}

func (f *uiFileDiff) row(r uiDiffRow) {
	if len(f.Rows) >= uiDetailRows {
		f.More++
		return
	}
	f.Rows = append(f.Rows, r)
}

func hunkStart(s string) int {
	s = strings.TrimLeft(s, "+-")
	s, _, _ = strings.Cut(s, ",")
	n, _ := strconv.Atoi(s)
	return n
}

type uiBranchCommit struct {
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Rel     string `json:"rel"`
}

// uiBranchDetail is what the /switch pane draws: how the branch relates to the
// current one, what it changes, and its latest commits.
type uiBranchDetail struct {
	Kind    string           `json:"kind"`
	Ref     string           `json:"ref"`
	Current string           `json:"current"`
	IsCur   bool             `json:"isCurrent"`
	Ahead   int              `json:"ahead"`  // commits the branch has that the current one lacks
	Behind  int              `json:"behind"` // the other way round
	Files   []uiFileDiff     `json:"files"`  // changed against the merge base, no rows
	Add     int              `json:"add"`
	Del     int              `json:"del"`
	Commits []uiBranchCommit `json:"commits"`
	Err     string           `json:"err"`
}

func uiBranchOf(dir, ref string) uiBranchDetail {
	d := uiBranchDetail{Kind: "branch", Ref: ref, Files: []uiFileDiff{}, Commits: []uiBranchCommit{}}
	cur, _ := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	d.Current, d.IsCur = cur, cur == ref
	out, err := gitOutput(dir, "log", "-n", "30", "--format=%h%x1f%s%x1f%an%x1f%cr", ref, "--")
	if err != nil {
		d.Err = err.Error()
		return d
	}
	for _, l := range strings.Split(out, "\n") {
		if f := strings.SplitN(l, "\x1f", 4); len(f) == 4 {
			d.Commits = append(d.Commits, uiBranchCommit{f[0], f[1], f[2], f[3]})
		}
	}
	if d.IsCur || cur == "" {
		return d
	}
	if c, err := gitOutput(dir, "rev-list", "--left-right", "--count", cur+"..."+ref); err == nil {
		if a, b, ok := strings.Cut(strings.Join(strings.Fields(c), " "), " "); ok {
			d.Behind, _ = strconv.Atoi(a)
			d.Ahead, _ = strconv.Atoi(b)
		}
	}
	if ns, err := gitOutput(dir, "diff", "--numstat", cur+"..."+ref, "--"); err == nil {
		for _, l := range strings.Split(ns, "\n") {
			f := strings.SplitN(l, "\t", 3)
			if len(f) != 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0])
			r, _ := strconv.Atoi(f[1])
			d.Add += a
			d.Del += r
			if len(d.Files) < uiDetailFiles {
				d.Files = append(d.Files, uiFileDiff{Path: f[2], Add: a, Del: r, Rows: []uiDiffRow{}})
			}
		}
	}
	return d
}
